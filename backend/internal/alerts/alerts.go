package alerts

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ DB *pgxpool.Pool }

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, message string) {
	reply(w, code, map[string]any{"error": map[string]string{"message": message}})
}

func (h *Handler) Rules(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}

	if r.Method == "GET" {
		rows, err := h.DB.Query(r.Context(), `
   SELECT a.id::text,a.name,COALESCE(p.rule,'off')
   FROM applications a
   LEFT JOIN application_alert_rules p ON p.application_id=a.id
   WHERE a.user_id=$1 ORDER BY a.name`, id)
		if err != nil {
			fail(w, 500, "Rules unavailable.")
			return
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var app, name, rule string
			if rows.Scan(&app, &name, &rule) != nil {
				fail(w, 500, "Rules unavailable.")
				return
			}
			items = append(items, map[string]string{"id": app, "name": name, "rule": rule})
		}
		if rows.Err() != nil {
			fail(w, 500, "Rules unavailable.")
			return
		}
		reply(w, 200, map[string]any{"applications": items})
		return
	}

	app, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		fail(w, 400, "Invalid application.")
		return
	}
	var input struct {
		Rule string `json:"rule"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	err = d.Decode(&input)
	var extra any
	if err != nil || d.Decode(&extra) != io.EOF ||
		(input.Rule != "off" && input.Rule != "critical" && input.Rule != "all") {
		fail(w, 400, "Select a valid alert rule.")
		return
	}

	tag, err := h.DB.Exec(r.Context(), `
 INSERT INTO application_alert_rules(application_id,rule)
 SELECT id,$3 FROM applications WHERE id=$1 AND user_id=$2
 ON CONFLICT(application_id) DO UPDATE SET rule=EXCLUDED.rule,
 enabled_at=CASE WHEN application_alert_rules.rule IS DISTINCT FROM EXCLUDED.rule
 THEN now() ELSE application_alert_rules.enabled_at END`, app, id, input.Rule)
	if err != nil {
		fail(w, 500, "Rule could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Application not found.")
		return
	}
	reply(w, 200, map[string]bool{"saved": true})
}

func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
 SELECT i.id::text,a.name,i.title,i.severity,i.status,i.created_at
 FROM incidents i
 JOIN applications a ON a.id=i.application_id
 JOIN application_alert_rules p ON p.application_id=a.id
 WHERE a.user_id=$1 AND p.rule!='off'
 AND (p.rule='all' OR i.severity='critical')
 AND i.created_at>=p.enabled_at
 ORDER BY i.created_at DESC LIMIT 50`, id)
	if err != nil {
		fail(w, 500, "Alerts unavailable.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var iid, name, title, severity, status string
		var created time.Time
		if rows.Scan(&iid, &name, &title, &severity, &status, &created) != nil {
			fail(w, 500, "Alerts unavailable.")
			return
		}
		items = append(items, map[string]any{
			"id": iid, "name": name, "title": title, "severity": severity,
			"status": status, "created_at": created,
		})
	}
	if rows.Err() != nil {
		fail(w, 500, "Alerts unavailable.")
		return
	}
	reply(w, 200, map[string]any{"alerts": items})
}
