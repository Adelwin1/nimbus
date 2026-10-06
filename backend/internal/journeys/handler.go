package journeys

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/adel/nimbus/backend/internal/githubapp"
	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB     *pgxpool.Pool
	GitHub *githubapp.Handler
}

type Step struct {
	Action   string  `json:"action"`
	Path     string  `json:"path,omitempty"`
	Selector string  `json:"selector,omitempty"`
	Value    *string `json:"value,omitempty"`
}
type Definition struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Steps   []Step `json:"steps"`
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"message": message}})
}
func valid(d *Definition) bool {
	d.Name = strings.TrimSpace(d.Name)
	if !utf8.ValidString(d.Name) || len([]rune(d.Name)) < 1 ||
		len([]rune(d.Name)) > 120 || len(d.BaseURL) > 2048 {
		return false
	}
	base, err := url.Parse(d.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") ||
		base.Hostname() == "" || base.User != nil || base.Fragment != "" ||
		base.RawQuery != "" || len(d.Steps) < 1 || len(d.Steps) > 20 {
		return false
	}
	for _, s := range d.Steps {
		if len(s.Selector) > 512 || len(s.Path) > 2048 ||
			(s.Value != nil && len(*s.Value) > 4096) {
			return false
		}
		switch s.Action {
		case "navigate":
			p, err := url.Parse(s.Path)
			if err != nil || s.Path == "" || s.Selector != "" || s.Value != nil {
				return false
			}
			target := base.ResolveReference(p)
			if target.Scheme != base.Scheme || target.Host != base.Host ||
				target.User != nil {
				return false
			}
		case "click", "expect_visible":
			if strings.TrimSpace(s.Selector) == "" || s.Path != "" || s.Value != nil {
				return false
			}
		case "fill", "expect_text":
			if strings.TrimSpace(s.Selector) == "" || s.Path != "" || s.Value == nil {
				return false
			}
		default:
			return false
		}
	}
	return d.Steps[0].Action == "navigate"
}

func (h *Handler) Applications(w http.ResponseWriter, r *http.Request) {
	user, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	app, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		fail(w, 400, "Invalid application.")
		return
	}
	var exists bool
	err = h.DB.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applications WHERE id=$1 AND user_id=$2)`,
		app, user).Scan(&exists)
	if err != nil {
		fail(w, 500, "Journeys unavailable.")
		return
	}
	if !exists {
		fail(w, 404, "Application not found.")
		return
	}
	if r.Method == http.MethodGet {
		var body []byte
		err = h.DB.QueryRow(r.Context(), `
			SELECT COALESCE(jsonb_agg(to_jsonb(j)), '[]'::jsonb)
			FROM (SELECT id,name,base_url,steps,enabled,created_at,updated_at
			      FROM browser_journeys WHERE application_id=$1
			      ORDER BY created_at DESC LIMIT 50) j`, app).Scan(&body)
		if err != nil {
			fail(w, 500, "Journeys unavailable.")
			return
		}
		reply(w, 200, map[string]any{"journeys": json.RawMessage(body)})
		return
	}
	var d Definition
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&d) != nil || decoder.Decode(&extra) != io.EOF || !valid(&d) {
		fail(w, 400, "Invalid journey. Begin with navigation; use 1–20 supported steps and one origin.")
		return
	}
	steps, _ := json.Marshal(d.Steps)
	id := uuid.New()
	tag, err := h.DB.Exec(r.Context(), `
		INSERT INTO browser_journeys(id,application_id,name,base_url,steps)
		SELECT $3,id,$4,$5,$6::jsonb FROM applications WHERE id=$1 AND user_id=$2`,
		app, user, id, d.Name, d.BaseURL, steps)
	if err != nil {
		fail(w, 500, "Journey could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 201, map[string]any{"id": id, "saved": true})
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	user, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	journey, err := uuid.Parse(chi.URLParam(r, "journeyID"))
	if err != nil {
		fail(w, 400, "Invalid journey.")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Journey runs unavailable.")
		return
	}
	defer tx.Rollback(r.Context())

	var definition []byte
	var enabled bool
	err = tx.QueryRow(r.Context(), `
		SELECT jsonb_build_object('name',j.name,'base_url',j.base_url,'steps',j.steps),
		       j.enabled
		FROM browser_journeys j JOIN applications a ON a.id=j.application_id
		WHERE j.id=$1 AND a.user_id=$2 FOR UPDATE OF j`, journey, user).
		Scan(&definition, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Journey not found.")
		return
	}
	if err != nil {
		fail(w, 500, "Journey runs unavailable.")
		return
	}
	if r.Method == http.MethodGet {
		var body []byte
		err = tx.QueryRow(r.Context(), `
			SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb)
			FROM (SELECT id,status,result,error_message,queued_at,started_at,finished_at,release_context,source_investigation
			      FROM browser_journey_runs WHERE journey_id=$1
			      ORDER BY queued_at DESC LIMIT 25) x`, journey).Scan(&body)
		if err != nil {
			fail(w, 500, "Journey runs unavailable.")
			return
		}
		reply(w, 200, map[string]any{"runs": json.RawMessage(body)})
		return
	}
	if !enabled {
		fail(w, 409, "Journey is disabled.")
		return
	}
	var busy bool
	err = tx.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM browser_journey_runs
		              WHERE journey_id=$1 AND status IN ('queued','running'))`,
		journey).Scan(&busy)
	if err != nil {
		fail(w, 500, "Run could not be queued.")
		return
	}
	if busy {
		fail(w, 409, "This journey already has a queued or running test.")
		return
	}

	var input RunInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	decodeErr := decoder.Decode(&input)
	var extra any
	if (decodeErr != nil && decodeErr != io.EOF) || (decodeErr == nil && decoder.Decode(&extra) != io.EOF) || !validRelease(&input) {
		fail(w, 400, "Provide a full lowercase 40-character commit SHA and a public HTTPS preview origin, or leave both empty.")
		return
	}
	release := map[string]any{}
	if input.CommitSHA != "" {
		if h.GitHub == nil {
			fail(w, 503, "Release tracking is unavailable.")
			return
		}
		var app uuid.UUID
		if tx.QueryRow(r.Context(), `SELECT application_id FROM browser_journeys WHERE id=$1`, journey).Scan(&app) != nil {
			fail(w, 500, "Run could not be queued.")
			return
		}
		release, err = h.GitHub.VerifyCommit(r, app, input.CommitSHA)
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		var d Definition
		if json.Unmarshal(definition, &d) != nil {
			fail(w, 500, "Journey definition unavailable.")
			return
		}
		d.BaseURL = input.PreviewURL
		if !valid(&d) {
			fail(w, 400, "Journey contains navigation to another origin. Use relative navigation paths for preview runs.")
			return
		}
		definition, _ = json.Marshal(d)
		release["preview_url"] = input.PreviewURL
	}
	releaseJSON, _ := json.Marshal(release)
	id := uuid.New()
	_, err = tx.Exec(r.Context(), `
		INSERT INTO browser_journey_runs(id,journey_id,definition,release_context)
		VALUES($1,$2,$3::jsonb,$4::jsonb)`, id, journey, definition, releaseJSON)
	if err != nil {
		fail(w, 500, "Run could not be queued.")
		return
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "Run could not be queued.")
		return
	}
	reply(w, 202, map[string]any{
		"id": id, "status": "queued", "queued_at": time.Now().UTC(),
	})
}
