package live

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adel/nimbus/backend/internal/httpx"
	"net/http"
	"time"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPollInterval  = 2 * time.Second
	defaultHeartbeat     = 15 * time.Second
	initialHealthLimit   = 25
	initialEventLimit    = 50
	initialIncidentLimit = 50
)

type Handler struct {
	db            *pgxpool.Pool
	pollInterval  time.Duration
	heartbeatTime time.Duration
}

type eventCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

type HealthEvent struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	StatusCode    *int      `json:"status_code"`
	LatencyMS     int64     `json:"latency_ms"`
	Healthy       bool      `json:"healthy"`
	ErrorMessage  *string   `json:"error_message"`
	CheckedAt     time.Time `json:"checked_at"`
}

type DeploymentEvent struct {
	ID             uuid.UUID       `json:"id"`
	ApplicationID  uuid.UUID       `json:"application_id"`
	DeploymentID   uuid.UUID       `json:"deployment_id"`
	Version        string          `json:"version"`
	DeploymentType string          `json:"deployment_type"`
	Status         string          `json:"status"`
	EventType      string          `json:"event_type"`
	Message        string          `json:"message"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
}

type IncidentEvent struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	IncidentID    uuid.UUID `json:"incident_id"`

	IncidentType string `json:"incident_type"`
	Severity     string `json:"severity"`
	Status       string `json:"status"`
	Title        string `json:"title"`

	EventType string `json:"event_type"`
	Message   string `json:"message"`

	ActorUserID   *uuid.UUID `json:"actor_user_id"`
	HealthCheckID *uuid.UUID `json:"health_check_id"`
	DeploymentID  *uuid.UUID `json:"deployment_id"`

	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db:            db,
		pollInterval:  defaultPollInterval,
		heartbeatTime: defaultHeartbeat,
	}
}

func (h *Handler) StreamApplication(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := appmiddleware.GetUserID(r.Context())
	if !ok {
		writeError(
			w,
			r,
			http.StatusUnauthorized,
			"unauthorized",
			"Authentication is required.",
		)
		return
	}

	applicationID, err := uuid.Parse(
		chi.URLParam(r, "appID"),
	)
	if err != nil {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_application_id",
			"Application ID must be a valid UUID.",
		)
		return
	}

	owned, err := h.applicationOwned(
		r.Context(),
		applicationID,
		userID,
	)
	if err != nil {
		writeError(
			w,
			r,
			http.StatusInternalServerError,
			"internal_server_error",
			"Application ownership could not be verified.",
		)
		return
	}

	if !owned {
		writeError(
			w,
			r,
			http.StatusNotFound,
			"application_not_found",
			"Application not found.",
		)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(
			w,
			r,
			http.StatusInternalServerError,
			"streaming_unsupported",
			"Live event streaming is unavailable.",
		)
		return
	}

	setSSEHeaders(w)

	if err := sendEvent(
		w,
		"connected",
		uuid.NewString(),
		map[string]any{
			"application_id": applicationID,
			"connected_at":   time.Now().UTC(),
		},
	); err != nil {
		return
	}

	healthCursor, err := h.sendInitialHealthEvents(
		r.Context(),
		w,
		applicationID,
	)
	if err != nil {
		_ = sendStreamError(
			w,
			"Initial health events could not be loaded.",
		)
		flusher.Flush()
		return
	}

	deploymentCursor, err :=
		h.sendInitialDeploymentEvents(
			r.Context(),
			w,
			applicationID,
		)
	if err != nil {
		_ = sendStreamError(
			w,
			"Initial deployment events could not be loaded.",
		)
		flusher.Flush()
		return
	}

	incidentCursor, err :=
		h.sendInitialIncidentEvents(
			r.Context(),
			w,
			applicationID,
		)
	if err != nil {
		_ = sendStreamError(
			w,
			"Initial incident events could not be loaded.",
		)
		flusher.Flush()
		return
	}

	flusher.Flush()

	pollTicker := time.NewTicker(h.pollInterval)
	defer pollTicker.Stop()

	heartbeatTicker := time.NewTicker(h.heartbeatTime)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case <-heartbeatTicker.C:
			if _, err := fmt.Fprintf(
				w,
				": heartbeat %s\n\n",
				time.Now().UTC().Format(time.RFC3339),
			); err != nil {
				return
			}

			flusher.Flush()

		case <-pollTicker.C:
			newHealthCursor, err :=
				h.sendNewHealthEvents(
					r.Context(),
					w,
					applicationID,
					healthCursor,
				)
			if err != nil {
				_ = sendStreamError(
					w,
					"Health updates could not be loaded.",
				)
				flusher.Flush()
				return
			}

			healthCursor = newHealthCursor

			newDeploymentCursor, err :=
				h.sendNewDeploymentEvents(
					r.Context(),
					w,
					applicationID,
					deploymentCursor,
				)
			if err != nil {
				_ = sendStreamError(
					w,
					"Deployment updates could not be loaded.",
				)
				flusher.Flush()
				return
			}

			deploymentCursor = newDeploymentCursor

			newIncidentCursor, err :=
				h.sendNewIncidentEvents(
					r.Context(),
					w,
					applicationID,
					incidentCursor,
				)
			if err != nil {
				_ = sendStreamError(
					w,
					"Incident updates could not be loaded.",
				)
				flusher.Flush()
				return
			}

			incidentCursor = newIncidentCursor
			flusher.Flush()
		}
	}
}

func (h *Handler) applicationOwned(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM applications
			WHERE id = $1
			  AND user_id = $2
		)
	`

	var owned bool

	if err := h.db.QueryRow(
		ctx,
		query,
		applicationID,
		userID,
	).Scan(&owned); err != nil {
		return false, fmt.Errorf(
			"verify live-stream ownership: %w",
			err,
		)
	}

	return owned, nil
}

func (h *Handler) sendInitialHealthEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
) (eventCursor, error) {
	const query = `
		SELECT
			id,
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
		FROM (
			SELECT
				id,
				application_id,
				status_code,
				latency_ms,
				healthy,
				error_message,
				checked_at
			FROM health_checks
			WHERE application_id = $1
			ORDER BY checked_at DESC, id DESC
			LIMIT $2
		) AS latest_checks
		ORDER BY checked_at ASC, id ASC
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		initialHealthLimit,
	)
	if err != nil {
		return eventCursor{}, fmt.Errorf(
			"query initial health events: %w",
			err,
		)
	}
	defer rows.Close()

	cursor := emptyCursor()

	for rows.Next() {
		event, err := scanHealthEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"health",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CheckedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate initial health events: %w",
			err,
		)
	}

	return cursor, nil
}

func (h *Handler) sendNewHealthEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
	cursor eventCursor,
) (eventCursor, error) {
	const query = `
		SELECT
			id,
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
		FROM health_checks
		WHERE application_id = $1
		  AND (
				checked_at > $2
				OR (
					checked_at = $2
					AND id > $3
				)
		  )
		ORDER BY checked_at ASC, id ASC
		LIMIT 100
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		cursor.CreatedAt,
		cursor.ID,
	)
	if err != nil {
		return cursor, fmt.Errorf(
			"query new health events: %w",
			err,
		)
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanHealthEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"health",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CheckedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate new health events: %w",
			err,
		)
	}

	return cursor, nil
}

func (h *Handler) sendInitialDeploymentEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
) (eventCursor, error) {
	const query = `
		SELECT
			deployment_events.id,
			deployments.application_id,
			deployments.id,
			deployments.version,
			deployments.deployment_type,
			deployments.status,
			deployment_events.event_type,
			deployment_events.message,
			deployment_events.metadata,
			deployment_events.created_at
		FROM (
			SELECT
				deployment_events.id
			FROM deployment_events
			INNER JOIN deployments
				ON deployments.id =
					deployment_events.deployment_id
			WHERE deployments.application_id = $1
			ORDER BY
				deployment_events.created_at DESC,
				deployment_events.id DESC
			LIMIT $2
		) AS latest_events
		INNER JOIN deployment_events
			ON deployment_events.id = latest_events.id
		INNER JOIN deployments
			ON deployments.id =
				deployment_events.deployment_id
		ORDER BY
			deployment_events.created_at ASC,
			deployment_events.id ASC
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		initialEventLimit,
	)
	if err != nil {
		return eventCursor{}, fmt.Errorf(
			"query initial deployment events: %w",
			err,
		)
	}
	defer rows.Close()

	cursor := emptyCursor()

	for rows.Next() {
		event, err := scanDeploymentEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"deployment",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CreatedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate initial deployment events: %w",
			err,
		)
	}

	return cursor, nil
}

func (h *Handler) sendNewDeploymentEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
	cursor eventCursor,
) (eventCursor, error) {
	const query = `
		SELECT
			deployment_events.id,
			deployments.application_id,
			deployments.id,
			deployments.version,
			deployments.deployment_type,
			deployments.status,
			deployment_events.event_type,
			deployment_events.message,
			deployment_events.metadata,
			deployment_events.created_at
		FROM deployment_events
		INNER JOIN deployments
			ON deployments.id =
				deployment_events.deployment_id
		WHERE deployments.application_id = $1
		  AND (
				deployment_events.created_at > $2
				OR (
					deployment_events.created_at = $2
					AND deployment_events.id > $3
				)
		  )
		ORDER BY
			deployment_events.created_at ASC,
			deployment_events.id ASC
		LIMIT 100
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		cursor.CreatedAt,
		cursor.ID,
	)
	if err != nil {
		return cursor, fmt.Errorf(
			"query new deployment events: %w",
			err,
		)
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanDeploymentEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"deployment",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CreatedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate new deployment events: %w",
			err,
		)
	}

	return cursor, nil
}

func (h *Handler) sendInitialIncidentEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
) (eventCursor, error) {
	const query = `
		SELECT
			incident_events.id,
			incidents.application_id,
			incidents.id,
			incidents.incident_type,
			incidents.severity,
			incidents.status,
			incidents.title,
			incident_events.event_type,
			incident_events.message,
			incident_events.actor_user_id,
			incident_events.health_check_id,
			incident_events.deployment_id,
			incident_events.metadata,
			incident_events.created_at
		FROM (
			SELECT
				incident_events.id
			FROM incident_events
			INNER JOIN incidents
				ON incidents.id =
					incident_events.incident_id
			WHERE incidents.application_id = $1
			ORDER BY
				incident_events.created_at DESC,
				incident_events.id DESC
			LIMIT $2
		) AS latest_events
		INNER JOIN incident_events
			ON incident_events.id =
				latest_events.id
		INNER JOIN incidents
			ON incidents.id =
				incident_events.incident_id
		ORDER BY
			incident_events.created_at ASC,
			incident_events.id ASC
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		initialIncidentLimit,
	)
	if err != nil {
		return eventCursor{}, fmt.Errorf(
			"query initial incident events: %w",
			err,
		)
	}
	defer rows.Close()

	cursor := emptyCursor()

	for rows.Next() {
		event, err := scanIncidentEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"incident",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CreatedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate initial incident events: %w",
			err,
		)
	}

	return cursor, nil
}

func (h *Handler) sendNewIncidentEvents(
	ctx context.Context,
	w http.ResponseWriter,
	applicationID uuid.UUID,
	cursor eventCursor,
) (eventCursor, error) {
	const query = `
		SELECT
			incident_events.id,
			incidents.application_id,
			incidents.id,
			incidents.incident_type,
			incidents.severity,
			incidents.status,
			incidents.title,
			incident_events.event_type,
			incident_events.message,
			incident_events.actor_user_id,
			incident_events.health_check_id,
			incident_events.deployment_id,
			incident_events.metadata,
			incident_events.created_at
		FROM incident_events
		INNER JOIN incidents
			ON incidents.id =
				incident_events.incident_id
		WHERE incidents.application_id = $1
		  AND (
				incident_events.created_at > $2
				OR (
					incident_events.created_at = $2
					AND incident_events.id > $3
				)
		  )
		ORDER BY
			incident_events.created_at ASC,
			incident_events.id ASC
		LIMIT 100
	`

	rows, err := h.db.Query(
		ctx,
		query,
		applicationID,
		cursor.CreatedAt,
		cursor.ID,
	)
	if err != nil {
		return cursor, fmt.Errorf(
			"query new incident events: %w",
			err,
		)
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanIncidentEvent(rows)
		if err != nil {
			return cursor, err
		}

		if err := sendEvent(
			w,
			"incident",
			event.ID.String(),
			event,
		); err != nil {
			return cursor, err
		}

		cursor = eventCursor{
			CreatedAt: event.CreatedAt,
			ID:        event.ID,
		}
	}

	if err := rows.Err(); err != nil {
		return cursor, fmt.Errorf(
			"iterate new incident events: %w",
			err,
		)
	}

	return cursor, nil
}

func scanIncidentEvent(
	scanner interface {
		Scan(destinations ...any) error
	},
) (IncidentEvent, error) {
	var event IncidentEvent

	err := scanner.Scan(
		&event.ID,
		&event.ApplicationID,
		&event.IncidentID,
		&event.IncidentType,
		&event.Severity,
		&event.Status,
		&event.Title,
		&event.EventType,
		&event.Message,
		&event.ActorUserID,
		&event.HealthCheckID,
		&event.DeploymentID,
		&event.Metadata,
		&event.CreatedAt,
	)
	if err != nil {
		return IncidentEvent{}, fmt.Errorf(
			"scan incident event: %w",
			err,
		)
	}

	return event, nil
}

func scanHealthEvent(
	scanner interface {
		Scan(destinations ...any) error
	},
) (HealthEvent, error) {
	var event HealthEvent

	err := scanner.Scan(
		&event.ID,
		&event.ApplicationID,
		&event.StatusCode,
		&event.LatencyMS,
		&event.Healthy,
		&event.ErrorMessage,
		&event.CheckedAt,
	)
	if err != nil {
		return HealthEvent{}, fmt.Errorf(
			"scan health event: %w",
			err,
		)
	}

	return event, nil
}

func scanDeploymentEvent(
	scanner interface {
		Scan(destinations ...any) error
	},
) (DeploymentEvent, error) {
	var event DeploymentEvent

	err := scanner.Scan(
		&event.ID,
		&event.ApplicationID,
		&event.DeploymentID,
		&event.Version,
		&event.DeploymentType,
		&event.Status,
		&event.EventType,
		&event.Message,
		&event.Metadata,
		&event.CreatedAt,
	)
	if err != nil {
		return DeploymentEvent{}, fmt.Errorf(
			"scan deployment event: %w",
			err,
		)
	}

	return event, nil
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set(
		"Content-Type",
		"text/event-stream",
	)
	w.Header().Set(
		"Cache-Control",
		"no-cache, no-transform",
	)
	w.Header().Set(
		"Connection",
		"keep-alive",
	)
	w.Header().Set(
		"X-Accel-Buffering",
		"no",
	)
}

func sendEvent(
	w http.ResponseWriter,
	eventType string,
	eventID string,
	data any,
) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf(
			"encode SSE event: %w",
			err,
		)
	}

	if eventID != "" {
		if _, err := fmt.Fprintf(
			w,
			"id: %s\n",
			eventID,
		); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(
		w,
		"event: %s\n",
		eventType,
	); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(
		w,
		"data: %s\n\n",
		encoded,
	); err != nil {
		return err
	}

	return nil
}

func sendStreamError(
	w http.ResponseWriter,
	message string,
) error {
	return sendEvent(
		w,
		"stream_error",
		uuid.NewString(),
		map[string]string{
			"message": message,
		},
	)
}

func emptyCursor() eventCursor {
	return eventCursor{
		CreatedAt: time.Unix(0, 0).UTC(),
		ID:        uuid.Nil,
	}
}

func writeError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
) {
	httpx.WriteError(
		w,
		status,
		code,
		message,
		appmiddleware.GetRequestID(r.Context()),
		nil,
	)
}
