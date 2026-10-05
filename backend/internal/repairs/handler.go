package repairs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }
type Check struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	ExitCode *int   `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}
type Validation struct {
	TestsRun     bool    `json:"tests_run"`
	ChecksPassed bool    `json:"checks_passed"`
	Checks       []Check `json:"checks"`
}
type Input struct {
	Rule       string     `json:"rule"`
	BaseCommit string     `json:"base_commit"`
	Patch      string     `json:"patch"`
	Validation Validation `json:"validation"`
}

var commit = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"message": message}})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 49152))
	d.DisallowUnknownFields()
	var extra any
	return d.Decode(value) == nil && d.Decode(&extra) == io.EOF
}
func valid(input Input) bool {
	header := "diff --git a/backend/internal/middleware/cors.go b/backend/internal/middleware/cors.go\n"
	if input.Rule != "go-cors-route-method-mismatch" ||
		!commit.MatchString(input.BaseCommit) ||
		len(input.Patch) > 32768 ||
		!strings.HasPrefix(input.Patch, header) ||
		strings.Count(input.Patch, "diff --git ") != 1 ||
		!strings.Contains(input.Patch, "\n@@") ||
		len(input.Validation.Checks) > 2 {
		return false
	}
	names := map[string]bool{}
	allPassed := len(input.Validation.Checks) == 2
	hasTests := false
	for _, c := range input.Validation.Checks {
		if (c.Name != "middleware tests" && c.Name != "API build") || names[c.Name] {
			return false
		}
		names[c.Name] = true
		if c.Name == "middleware tests" {
			hasTests = true
		}
		if c.Passed && (c.ExitCode == nil || *c.ExitCode != 0 || c.TimedOut) {
			return false
		}
		allPassed = allPassed && c.Passed
	}
	return input.Validation.TestsRun == hasTests &&
		input.Validation.ChecksPassed == allPassed
}

func (h *Handler) Reviews(w http.ResponseWriter, r *http.Request) {
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
	err = h.DB.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM applications WHERE id=$1 AND user_id=$2)`,
		app, user).Scan(&exists)
	if err != nil {
		fail(w, 500, "Repair reviews unavailable.")
		return
	}
	if !exists {
		fail(w, 404, "Application not found.")
		return
	}
	if r.Method == http.MethodGet {
		var body []byte
		err = h.DB.QueryRow(r.Context(), `
			SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb)
			FROM (
			 SELECT id,rule,base_commit,patch,patch_sha256,validation,
			        review_status,created_at,reviewed_at
			 FROM application_repair_reviews WHERE application_id=$1
			 ORDER BY created_at DESC LIMIT 10
			) x`, app).Scan(&body)
		if err != nil {
			fail(w, 500, "Repair reviews unavailable.")
			return
		}
		reply(w, 200, map[string]any{"reviews": json.RawMessage(body)})
		return
	}
	var input Input
	if !decode(w, r, &input) || !valid(input) {
		fail(w, 400, "Invalid CORS repair proposal or inconsistent check results.")
		return
	}
	hash := sha256.Sum256([]byte(input.Patch))
	validation, _ := json.Marshal(input.Validation)
	id := uuid.New()
	tag, err := h.DB.Exec(r.Context(), `
		INSERT INTO application_repair_reviews(
		  id,application_id,rule,base_commit,patch,patch_sha256,validation)
		SELECT $3,id,$4,$5,$6,$7,$8::jsonb
		FROM applications WHERE id=$1 AND user_id=$2`,
		app, user, id, input.Rule, input.BaseCommit, input.Patch,
		hex.EncodeToString(hash[:]), validation)
	if err != nil {
		fail(w, 500, "Repair proposal could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 201, map[string]any{"id": id, "review_status": "pending"})
}

func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	user, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "reviewID"))
	if err != nil {
		fail(w, 400, "Invalid review.")
		return
	}
	var input struct {
		Decision string `json:"decision"`
	}
	if !decode(w, r, &input) ||
		(input.Decision != "approved" && input.Decision != "rejected") {
		fail(w, 400, "Choose approved or rejected.")
		return
	}
	var exists bool
	err = h.DB.QueryRow(r.Context(), `
		SELECT EXISTS(
		 SELECT 1 FROM application_repair_reviews x
		 JOIN applications a ON a.id=x.application_id
		 WHERE x.id=$1 AND a.user_id=$2)`, id, user).Scan(&exists)
	if err != nil {
		fail(w, 500, "Review decision could not be saved.")
		return
	}
	if !exists {
		fail(w, 404, "Review not found.")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE application_repair_reviews x
		SET review_status=$3,reviewed_at=now()
		FROM applications a
		WHERE x.id=$1 AND a.id=x.application_id AND a.user_id=$2
		  AND x.review_status='pending'`, id, user, input.Decision)
	if err != nil {
		fail(w, 500, "Review decision could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "This review already has a decision.")
		return
	}
	reply(w, 200, map[string]any{"review_status": input.Decision})
}
