package publicstatus

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,79}$`)

func reply(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, code int, message string) {
	reply(w, code, map[string]any{"error": map[string]string{"message": message}})
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
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

	if r.Method == "GET" {
		var slug string
		err = h.DB.QueryRow(r.Context(), `
   SELECT COALESCE(p.slug,'') FROM applications a
   LEFT JOIN public_status_pages p ON p.application_id=a.id
   WHERE a.id=$1 AND a.user_id=$2
  `, app, user).Scan(&slug)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Application not found.")
			return
		}
		if err != nil {
			fail(w, 500, "Settings unavailable.")
			return
		}
		reply(w, 200, map[string]string{"slug": slug})
		return
	}

	var input struct {
		Slug string `json:"slug"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	err = d.Decode(&input)
	var extra any
	if err != nil || d.Decode(&extra) != io.EOF {
		fail(w, 400, "Invalid request.")
		return
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if input.Slug != "" && !slugPattern.MatchString(input.Slug) {
		fail(w, 400, "Use 3–80 lowercase letters, numbers, or hyphens.")
		return
	}

	tag, err := h.DB.Exec(r.Context(), `
  INSERT INTO public_status_pages(application_id,slug)
  SELECT id,NULLIF($3,'') FROM applications WHERE id=$1 AND user_id=$2
  ON CONFLICT(application_id) DO UPDATE SET slug=EXCLUDED.slug
 `, app, user, input.Slug)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		fail(w, 409, "That slug is already in use.")
		return
	}
	if err != nil {
		fail(w, 500, "Settings could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 200, map[string]string{"slug": input.Slug})
}

func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !slugPattern.MatchString(slug) {
		fail(w, 404, "Status page not found.")
		return
	}
	var name, status string
	var checked *time.Time
	err := h.DB.QueryRow(r.Context(), `
  SELECT a.name,a.status,a.last_checked_at FROM applications a
  JOIN public_status_pages p ON p.application_id=a.id
  WHERE p.slug=$1
 `, slug).Scan(&name, &status, &checked)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Status page not found.")
		return
	}
	if err != nil {
		fail(w, 500, "Status unavailable.")
		return
	}
	reply(w, 200, map[string]any{
		"name": name, "status": status, "last_checked_at": checked,
		"generated_at": time.Now().UTC(),
	})
}
