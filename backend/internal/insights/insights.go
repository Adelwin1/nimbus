// Package insights provides bounded, tenant-scoped reliability analytics.
package insights

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/adel/nimbus/backend/internal/httpx"
	middleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Metrics struct {
	Checks         int64    `json:"checks"`
	Successful     int64    `json:"successful"`
	SuccessPercent *float64 `json:"success_percent"`
	AverageLatency *float64 `json:"average_latency_ms"`
	P95Latency     *float64 `json:"p95_latency_ms"`
}
type Bucket struct {
	Time       time.Time `json:"time"`
	Checks     int64     `json:"checks"`
	Successful int64     `json:"successful"`
	Latency    *float64  `json:"latency_ms"`
}
type Release struct {
	ID              uuid.UUID `json:"id"`
	ApplicationName string    `json:"application_name"`
	Version         string    `json:"version"`
	PreviousVersion *string   `json:"previous_version"`
	Status          string    `json:"status"`
	Kind            string    `json:"kind"`
	CreatedAt       time.Time `json:"created_at"`
	DurationSeconds *float64  `json:"duration_seconds"`
}
type Incident struct {
	ID              uuid.UUID `json:"id"`
	ApplicationName string    `json:"application_name"`
	Title           string    `json:"title"`
	Status          string    `json:"status"`
	Severity        string    `json:"severity"`
	CreatedAt       time.Time `json:"created_at"`
	DurationSeconds float64   `json:"duration_seconds"`
}
type Overview struct {
	WindowStart     time.Time  `json:"window_start"`
	GeneratedAt     time.Time  `json:"generated_at"`
	Metrics         Metrics    `json:"metrics"`
	History         []Bucket   `json:"history"`
	Releases        []Release  `json:"releases"`
	Incidents       []Incident `json:"incidents"`
	ActiveIncidents int64      `json:"active_incidents"`
}

type Store interface {
	Get(context.Context, uuid.UUID, time.Time) (Overview, error)
}
type Repository struct{ DB *pgxpool.Pool }

const MetricsSQL = `SELECT COUNT(*), COUNT(*) FILTER (WHERE h.healthy),
 AVG(h.latency_ms) FILTER (WHERE h.healthy),
 percentile_cont(0.95) WITHIN GROUP (ORDER BY h.latency_ms) FILTER (WHERE h.healthy)
 FROM health_checks h JOIN applications a ON a.id=h.application_id
 WHERE a.user_id=$1 AND h.checked_at >= $2 AND h.checked_at <= $3`
const HistorySQL = `WITH hours AS (
 SELECT generate_series(date_trunc('hour',$2::timestamptz), date_trunc('hour',$3::timestamptz), interval '1 hour') AS time
 ), checks AS (
 SELECT h.checked_at,h.healthy,h.latency_ms FROM health_checks h JOIN applications a ON a.id=h.application_id
 WHERE a.user_id=$1 AND h.checked_at >= $2 AND h.checked_at <= $3
 ) SELECT hours.time,COUNT(checks.checked_at),COUNT(checks.checked_at) FILTER (WHERE checks.healthy),
 AVG(checks.latency_ms) FILTER (WHERE checks.healthy)
 FROM hours LEFT JOIN checks ON date_trunc('hour',checks.checked_at)=hours.time
 GROUP BY hours.time ORDER BY hours.time`
const ReleasesSQL = `SELECT d.id,a.name,d.version,d.previous_version,d.status,d.deployment_type,d.created_at,
 CASE WHEN d.started_at IS NOT NULL AND d.completed_at IS NOT NULL
 THEN GREATEST(0,EXTRACT(EPOCH FROM d.completed_at-d.started_at))::float8 ELSE NULL END
 FROM deployments d JOIN applications a ON a.id=d.application_id
 WHERE a.user_id=$1 ORDER BY d.created_at DESC LIMIT 10`
const IncidentsSQL = `SELECT i.id,a.name,i.title,i.status,i.severity,i.created_at,
 GREATEST(0,EXTRACT(EPOCH FROM COALESCE(i.resolved_at,$2::timestamptz)-i.created_at))::float8
 FROM incidents i JOIN applications a ON a.id=i.application_id
 WHERE a.user_id=$1 ORDER BY (i.status='resolved'),i.created_at DESC LIMIT 10`

func (r *Repository) Get(ctx context.Context, userID uuid.UUID, now time.Time) (Overview, error) {
	result := Overview{WindowStart: now.Add(-24 * time.Hour), GeneratedAt: now, History: []Bucket{}, Releases: []Release{}, Incidents: []Incident{}}
	err := r.DB.QueryRow(ctx, MetricsSQL, userID, result.WindowStart, now).Scan(&result.Metrics.Checks, &result.Metrics.Successful, &result.Metrics.AverageLatency, &result.Metrics.P95Latency)
	if err != nil {
		return Overview{}, err
	}
	if result.Metrics.Checks > 0 {
		percent := 100 * float64(result.Metrics.Successful) / float64(result.Metrics.Checks)
		result.Metrics.SuccessPercent = &percent
	}
	rows, err := r.DB.Query(ctx, HistorySQL, userID, result.WindowStart, now)
	if err != nil {
		return Overview{}, err
	}
	for rows.Next() {
		var bucket Bucket
		if err = rows.Scan(&bucket.Time, &bucket.Checks, &bucket.Successful, &bucket.Latency); err != nil {
			rows.Close()
			return Overview{}, err
		}
		result.History = append(result.History, bucket)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Overview{}, err
	}
	rows, err = r.DB.Query(ctx, ReleasesSQL, userID)
	if err != nil {
		return Overview{}, err
	}
	for rows.Next() {
		var release Release
		if err = rows.Scan(&release.ID, &release.ApplicationName, &release.Version, &release.PreviousVersion, &release.Status, &release.Kind, &release.CreatedAt, &release.DurationSeconds); err != nil {
			rows.Close()
			return Overview{}, err
		}
		result.Releases = append(result.Releases, release)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Overview{}, err
	}
	rows, err = r.DB.Query(ctx, IncidentsSQL, userID, now)
	if err != nil {
		return Overview{}, err
	}
	for rows.Next() {
		var incident Incident
		if err = rows.Scan(&incident.ID, &incident.ApplicationName, &incident.Title, &incident.Status, &incident.Severity, &incident.CreatedAt, &incident.DurationSeconds); err != nil {
			rows.Close()
			return Overview{}, err
		}
		result.Incidents = append(result.Incidents, incident)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Overview{}, err
	}
	err = r.DB.QueryRow(ctx, `SELECT COUNT(*) FROM incidents i JOIN applications a ON a.id=i.application_id WHERE a.user_id=$1 AND i.status<>'resolved'`, userID).Scan(&result.ActiveIncidents)
	return result, err
}

type Handler struct{ Store Store }

func (h Handler) Overview(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		httpx.Unauthorized(w, middleware.GetRequestID(r.Context()), "unauthorized", "Authentication is required.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := h.Store.Get(ctx, userID, time.Now().UTC())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "analytics_unavailable", "Reliability analytics could not be loaded.", middleware.GetRequestID(r.Context()), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	_ = json.NewEncoder(w).Encode(result)
}
