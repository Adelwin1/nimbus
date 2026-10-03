package incident

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) AddNote(ctx context.Context, incidentID, userID uuid.UUID, message string) (IncidentEvent, error) {
	event, err := scanIncidentEvent(r.db.QueryRow(ctx, `
		INSERT INTO incident_events (
			incident_id, event_type, message, actor_user_id, metadata
		)
		SELECT i.id, 'investigation_note', $3, $2, '{}'::jsonb
		FROM incidents i
		JOIN applications a ON a.id = i.application_id
		WHERE i.id = $1 AND a.user_id = $2
		RETURNING id, incident_id, event_type, message, actor_user_id,
			health_check_id, deployment_id, metadata, created_at
	`, incidentID, userID, message))

	if errors.Is(err, pgx.ErrNoRows) {
		return IncidentEvent{}, ErrIncidentNotFound
	}
	return event, err
}

func (h *Handler) AddNote(w http.ResponseWriter, r *http.Request) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}
	incidentID, ok := incidentUUIDParameter(
		w, r, "incidentID", "invalid_incident_id",
		"Incident ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	var input struct {
		Message string `json:"message"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&input)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("invalid JSON")
		}
	}
	input.Message = strings.TrimSpace(input.Message)
	length := utf8.RuneCountInString(input.Message)
	if err != nil || !utf8.ValidString(input.Message) || length < 1 || length > 5000 {
		writeIncidentJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]string{
				"code":    "invalid_note",
				"message": "Enter a note between 1 and 5,000 characters.",
			},
		})
		return
	}

	store, ok := h.service.repository.(interface {
		AddNote(context.Context, uuid.UUID, uuid.UUID, string) (IncidentEvent, error)
	})
	if !ok {
		h.handleError(w, r, errors.New("notes unavailable"))
		return
	}
	event, err := store.AddNote(r.Context(), incidentID, userID, input.Message)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeIncidentJSON(w, http.StatusCreated, map[string]any{"event": event})
}
