package incident

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/adel/nimbus/backend/internal/activity"
	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	service  *Service
	rollback *RollbackService
	activity activity.Recorder
}

func NewHandler(
	service *Service,
	rollback *RollbackService,
	activityRecorder activity.Recorder,
) *Handler {
	return &Handler{
		service:  service,
		rollback: rollback,
		activity: activityRecorder,
	}
}

func (h *Handler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	incidents, err := h.service.ListForUser(
		r.Context(),
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	writeIncidentJSON(
		w,
		http.StatusOK,
		IncidentListResponse{
			Incidents: incidents,
		},
	)
}

func (h *Handler) ListByApplication(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := incidentUUIDParameter(
		w,
		r,
		"appID",
		"invalid_application_id",
		"Application ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	incidents, err := h.service.ListByApplication(
		r.Context(),
		applicationID,
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	writeIncidentJSON(
		w,
		http.StatusOK,
		IncidentListResponse{
			Incidents: incidents,
		},
	)
}

func (h *Handler) Detail(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	incidentID, ok := incidentUUIDParameter(
		w,
		r,
		"incidentID",
		"invalid_incident_id",
		"Incident ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	detail, err := h.service.GetDetail(
		r.Context(),
		incidentID,
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	writeIncidentJSON(
		w,
		http.StatusOK,
		detail,
	)
}

func (h *Handler) Acknowledge(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	incidentID, ok := incidentUUIDParameter(
		w,
		r,
		"incidentID",
		"invalid_incident_id",
		"Incident ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	response, err := h.service.Acknowledge(
		r.Context(),
		incidentID,
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	userIDCopy := userID
	applicationIDCopy := response.Incident.ApplicationID
	incidentIDCopy := response.Incident.ID

	activity.RecordRequest(
		h.activity,
		r,
		activity.RequestRecordInput{
			ActorUserID:   &userIDCopy,
			ApplicationID: &applicationIDCopy,
			Action:        activity.ActionIncidentAcknowledged,
			EntityType:    activity.EntityIncident,
			EntityID:      &incidentIDCopy,
			Summary:       "Incident was acknowledged.",
			Metadata: map[string]any{
				"status": response.Incident.Status,
			},
		},
	)

	writeIncidentJSON(
		w,
		http.StatusOK,
		response,
	)
}

func (h *Handler) Resolve(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	incidentID, ok := incidentUUIDParameter(
		w,
		r,
		"incidentID",
		"invalid_incident_id",
		"Incident ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	response, err := h.service.Resolve(
		r.Context(),
		incidentID,
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	userIDCopy := userID
	applicationIDCopy := response.Incident.ApplicationID
	incidentIDCopy := response.Incident.ID

	activity.RecordRequest(
		h.activity,
		r,
		activity.RequestRecordInput{
			ActorUserID:   &userIDCopy,
			ApplicationID: &applicationIDCopy,
			Action:        activity.ActionIncidentResolved,
			EntityType:    activity.EntityIncident,
			EntityID:      &incidentIDCopy,
			Summary:       "Incident was resolved.",
			Metadata: map[string]any{
				"status": response.Incident.Status,
			},
		},
	)

	writeIncidentJSON(
		w,
		http.StatusOK,
		response,
	)
}

func (h *Handler) Rollback(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := incidentUserID(w, r)
	if !ok {
		return
	}

	incidentID, ok := incidentUUIDParameter(
		w,
		r,
		"incidentID",
		"invalid_incident_id",
		"Incident ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	rollback, err := h.rollback.Start(
		r.Context(),
		incidentID,
		userID,
	)
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	userIDCopy := userID
	applicationIDCopy := rollback.ApplicationID
	rollbackIDCopy := rollback.ID

	activity.RecordRequest(
		h.activity,
		r,
		activity.RequestRecordInput{
			ActorUserID:   &userIDCopy,
			ApplicationID: &applicationIDCopy,
			Action:        activity.ActionRollbackStarted,
			EntityType:    activity.EntityRollback,
			EntityID:      &rollbackIDCopy,
			Summary: fmt.Sprintf(
				"Rollback to version %s was started.",
				rollback.Version,
			),
			Metadata: map[string]any{
				"incident_id":     incidentID,
				"target_version":  rollback.Version,
				"deployment_type": rollback.DeploymentType,
			},
		},
	)

	writeIncidentJSON(
		w,
		http.StatusAccepted,
		map[string]any{
			"deployment": rollback,
		},
	)
}

func (h *Handler) handleError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, ErrIncidentNotFound):
		writeIncidentError(
			w,
			r,
			http.StatusNotFound,
			"incident_not_found",
			"Incident not found.",
		)

	case errors.Is(
		err,
		ErrIncidentAlreadyResolved,
	):
		writeIncidentError(
			w,
			r,
			http.StatusConflict,
			"incident_already_resolved",
			"The incident has already been resolved.",
		)

	case errors.Is(err, ErrInvalidIncidentState):
		writeIncidentError(
			w,
			r,
			http.StatusConflict,
			"invalid_incident_state",
			"The incident cannot perform this action in its current state.",
		)

	case errors.Is(err, ErrRollbackAlreadyLinked):
		writeIncidentError(
			w,
			r,
			http.StatusConflict,
			"rollback_already_linked",
			"A rollback deployment is already linked to this incident.",
		)

	case errors.Is(
		err,
		ErrRollbackWebhookNotConfigured,
	):
		writeIncidentError(
			w,
			r,
			http.StatusBadRequest,
			"rollback_webhook_not_configured",
			"Configure a rollback webhook before starting a rollback.",
		)

	case errors.Is(
		err,
		ErrNoPreviousHealthyDeployment,
	):
		writeIncidentError(
			w,
			r,
			http.StatusConflict,
			"no_previous_healthy_deployment",
			"Nimbus could not find a successful deployment to restore.",
		)

	case errors.Is(err, ErrRollbackInProgress):
		writeIncidentError(
			w,
			r,
			http.StatusConflict,
			"rollback_in_progress",
			"Another deployment or rollback is already active.",
		)

	default:
		writeIncidentError(
			w,
			r,
			http.StatusInternalServerError,
			"internal_server_error",
			"An unexpected error occurred.",
		)
	}
}

func incidentUserID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	userID, ok := appmiddleware.GetUserID(
		r.Context(),
	)
	if !ok {
		writeIncidentError(
			w,
			r,
			http.StatusUnauthorized,
			"unauthorized",
			"Authentication is required.",
		)

		return uuid.Nil, false
	}

	return userID, true
}

func incidentUUIDParameter(
	w http.ResponseWriter,
	r *http.Request,
	parameter string,
	code string,
	message string,
) (uuid.UUID, bool) {
	value := chi.URLParam(r, parameter)

	parsed, err := uuid.Parse(value)
	if err != nil {
		writeIncidentError(
			w,
			r,
			http.StatusBadRequest,
			code,
			message,
		)

		return uuid.Nil, false
	}

	return parsed, true
}

func writeIncidentJSON(
	w http.ResponseWriter,
	status int,
	payload any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)
	w.WriteHeader(status)

	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeIncidentError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
) {
	writeIncidentJSON(
		w,
		status,
		map[string]any{
			"error": map[string]string{
				"code":    code,
				"message": message,
				"request_id": appmiddleware.GetRequestID(
					r.Context(),
				),
			},
		},
	)
}
