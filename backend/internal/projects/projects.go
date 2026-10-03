package projects

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }
type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
	Status      string `json:"status"`
	Project     string `json:"project"`
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, message string) {
	reply(w, code, map[string]any{"error": map[string]string{"message": message}})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
 SELECT a.id::text,a.name,a.environment,a.status,
 COALESCE(p.project_name,'Default')
 FROM applications a
 LEFT JOIN application_projects p ON p.application_id=a.id
 WHERE a.user_id=$1
 ORDER BY COALESCE(p.project_name,'Default'),a.name`, id)
	if err != nil {
		fail(w, 500, "Projects unavailable.")
		return
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var item Item
		if rows.Scan(&item.ID, &item.Name, &item.Environment, &item.Status, &item.Project) != nil {
			fail(w, 500, "Projects unavailable.")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "Projects unavailable.")
		return
	}
	reply(w, 200, map[string]any{"applications": items})
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	app, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		fail(w, 400, "Invalid application.")
		return
	}
	var input struct {
		Project string `json:"project"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	err = d.Decode(&input)
	var extra any
	if err != nil || d.Decode(&extra) != io.EOF {
		fail(w, 400, "Invalid request.")
		return
	}
	input.Project = strings.TrimSpace(input.Project)
	length := utf8.RuneCountInString(input.Project)
	if !utf8.ValidString(input.Project) || length < 1 || length > 80 {
		fail(w, 400, "Project names must contain 1–80 characters.")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
 INSERT INTO application_projects(application_id,project_name)
 SELECT id,$3 FROM applications WHERE id=$1 AND user_id=$2
 ON CONFLICT(application_id) DO UPDATE SET project_name=EXCLUDED.project_name`,
		app, id, input.Project)
	if err != nil {
		fail(w, 500, "Project could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 200, map[string]bool{"saved": true})
}
