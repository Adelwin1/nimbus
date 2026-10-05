package apperrors

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }
type Frame struct {
	File     string `json:"file"`
	Function string `json:"function"`
	Line     int    `json:"line"`
}
type Event struct {
	Type    string  `json:"error_type"`
	Code    string  `json:"error_code"`
	Release string  `json:"release_version"`
	Frames  []Frame `json:"frames"`
}

var label = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)
var filename = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,160}$`)
var release = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,120}$`)

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"message": message}})
}
func valid(e *Event) bool {
	if !label.MatchString(e.Type) || !label.MatchString(e.Code) ||
		(e.Release != "" && !release.MatchString(e.Release)) ||
		len(e.Frames) > 20 {
		return false
	}
	for _, f := range e.Frames {
		if !filename.MatchString(f.File) || strings.HasPrefix(f.File, "/") ||
			strings.Contains(f.File, "..") || !label.MatchString(f.Function) ||
			f.Line < 1 || f.Line > 10000000 {
			return false
		}
	}
	if e.Frames == nil {
		e.Frames = []Frame{}
	}
	return true
}

func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
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
	random := make([]byte, 32)
	if _, err = rand.Read(random); err != nil {
		fail(w, 500, "Token could not be created.")
		return
	}
	token := "nimbus_err_" + base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(token))
	tag, err := h.DB.Exec(r.Context(), `
		INSERT INTO application_error_tokens(application_id,token_hash)
		SELECT id,$3 FROM applications WHERE id=$1 AND user_id=$2
		ON CONFLICT(application_id) DO UPDATE
		SET token_hash=EXCLUDED.token_hash,created_at=now()`,
		app, user, hash[:])
	if err != nil {
		fail(w, 500, "Token could not be created.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 201, map[string]any{
		"token":   token,
		"message": "Store this token in your application server environment. Creating another token invalidates this one.",
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 500, "Errors unavailable.")
		return
	}
	if !exists {
		fail(w, 404, "Application not found.")
		return
	}
	var body []byte
	err = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb)
		FROM (
		  SELECT g.id,g.error_type,g.error_code,g.title,g.frames,g.occurrences,
		         g.first_seen_at,g.last_seen_at,
		         (SELECT o.release_version FROM application_error_occurrences o
		          WHERE o.group_id=g.id ORDER BY o.received_at DESC,o.id DESC
		          LIMIT 1) AS latest_release
		  FROM application_error_groups g WHERE g.application_id=$1
		  ORDER BY g.last_seen_at DESC LIMIT 50
		) x`, app).Scan(&body)
	if err != nil {
		fail(w, 500, "Errors unavailable.")
		return
	}
	reply(w, 200, map[string]any{"errors": json.RawMessage(body)})
}

func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	app, err := uuid.Parse(chi.URLParam(r, "appID"))
	auth := r.Header.Get("Authorization")
	if err != nil || !strings.HasPrefix(auth, "Bearer nimbus_err_") ||
		len(auth) > 100 {
		fail(w, 401, "Invalid reporting credentials.")
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	hash := sha256.Sum256([]byte(token))
	var event Event
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&event) != nil || d.Decode(&extra) != io.EOF || !valid(&event) {
		fail(w, 400, "Invalid structured error. Use an error type, code, and optional relative stack frames.")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}
	defer tx.Rollback(r.Context())
	var stored []byte
	err = tx.QueryRow(r.Context(), `
		SELECT token_hash FROM application_error_tokens
		WHERE application_id=$1 FOR UPDATE`, app).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && subtle.ConstantTimeCompare(stored, hash[:]) != 1) {
		fail(w, 401, "Invalid reporting credentials.")
		return
	}
	if err != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}

	var count int
	err = tx.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM application_error_occurrences o
		JOIN application_error_groups g ON g.id=o.group_id
		WHERE g.application_id=$1 AND o.received_at > now()-interval '24 hours'`,
		app).Scan(&count)
	if err != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}
	if count >= 5000 {
		fail(w, 429, "Application error reporting limit reached.")
		return
	}

	frames, _ := json.Marshal(event.Frames)
	signature, _ := json.Marshal(struct {
		Type   string
		Code   string
		Frames []Frame
	}{event.Type, event.Code, event.Frames})
	fingerprint := sha256.Sum256(signature)
	var group uuid.UUID
	err = tx.QueryRow(r.Context(), `
		INSERT INTO application_error_groups(
		 id,application_id,fingerprint,error_type,error_code,title,frames)
		VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)
		ON CONFLICT(application_id,fingerprint) DO UPDATE
		SET occurrences=application_error_groups.occurrences+1,last_seen_at=now()
		RETURNING id`,
		uuid.New(), app, hex.EncodeToString(fingerprint[:]),
		event.Type, event.Code, event.Type+" · "+event.Code, frames).Scan(&group)
	if err != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO application_error_occurrences(id,group_id,release_version)
		VALUES($1,$2,NULLIF($3,''))`, uuid.New(), group, event.Release)
	if err != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}
	if tx.Commit(r.Context()) != nil {
		fail(w, 500, "Error could not be recorded.")
		return
	}
	reply(w, 202, map[string]any{"recorded": true, "group_id": group})
}
