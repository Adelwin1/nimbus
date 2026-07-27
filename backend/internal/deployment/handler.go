package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adel/nimbus/backend/internal/httpx"
	"io"
	"net/http"

	"github.com/adel/nimbus/backend/internal/activity"
	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	service  *Service
	activity activity.Recorder
}

func NewHandler(
	service *Service,
	activityRecorder activity.Recorder,
) *Handler {
	return &Handler{
		service:  service,
		activity: activityRecorder,
	}
}

func (h *Handler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := parseUUIDParameter(
		w,
		r,
		"appID",
		"invalid_application_id",
		"Application ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	var request CreateRequest

	if err := decodeJSON(r, &request); err != nil {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Request body contains invalid JSON.",
		)
		return
	}

	deployment, err := h.service.Create(
		r.Context(),
		applicationID,
		userID,
		request,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	userIDCopy := userID
	applicationIDCopy := applicationID
	deploymentIDCopy := deployment.ID

	activity.RecordRequest(
		h.activity,
		r,
		activity.RequestRecordInput{
			ActorUserID:   &userIDCopy,
			ApplicationID: &applicationIDCopy,
			Action:        activity.ActionDeploymentCreated,
			EntityType:    activity.EntityDeployment,
			EntityID:      &deploymentIDCopy,
			Summary: fmt.Sprintf(
				"Deployment %s was created.",
				deployment.Version,
			),
			Metadata: map[string]any{
				"version":         deployment.Version,
				"deployment_type": deployment.DeploymentType,
				"commit_sha":      deployment.CommitSHA,
			},
		},
	)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"deployment": deployment,
	})
}

func (h *Handler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := parseUUIDParameter(
		w,
		r,
		"appID",
		"invalid_application_id",
		"Application ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	deployments, err := h.service.List(
		r.Context(),
		applicationID,
		userID,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deployments": deployments,
	})
}

func (h *Handler) Detail(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	deploymentID, ok := parseUUIDParameter(
		w,
		r,
		"deploymentID",
		"invalid_deployment_id",
		"Deployment ID must be a valid UUID.",
	)
	if !ok {
		return
	}

	detail, err := h.service.GetDetail(
		r.Context(),
		deploymentID,
		userID,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) handleServiceError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_input",
			err.Error(),
		)

	case errors.Is(err, ErrApplicationNotFound):
		writeError(
			w,
			r,
			http.StatusNotFound,
			"application_not_found",
			"Application not found.",
		)

	case errors.Is(err, ErrDeploymentNotFound):
		writeError(
			w,
			r,
			http.StatusNotFound,
			"deployment_not_found",
			"Deployment not found.",
		)

	case errors.Is(err, ErrActiveDeployment):
		writeError(
			w,
			r,
			http.StatusConflict,
			"active_deployment",
			"This application already has an active deployment.",
		)

	case errors.Is(err, ErrWebhookNotConfigured):
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"webhook_not_configured",
			"Configure a deployment webhook before creating a deployment.",
		)

	case errors.Is(err, ErrUnsafeWebhookTarget):
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"unsafe_webhook_target",
			"The deployment webhook points to an unsafe destination.",
		)

	default:
		writeError(
			w,
			r,
			http.StatusInternalServerError,
			"internal_server_error",
			"An unexpected error occurred.",
		)
	}
}

func authenticatedUserID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	userID, ok := appmiddleware.GetUserID(r.Context())
	if !ok {
		writeError(
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

func parseUUIDParameter(
	w http.ResponseWriter,
	r *http.Request,
	parameter string,
	errorCode string,
	errorMessage string,
) (uuid.UUID, bool) {
	value := chi.URLParam(r, parameter)

	parsed, err := uuid.Parse(value)
	if err != nil {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			errorCode,
			errorMessage,
		)
		return uuid.Nil, false
	}

	return parsed, true
}

func decodeJSON(
	r *http.Request,
	destination any,
) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New(
			"request body must contain one JSON object",
		)
	}

	return nil
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	payload any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
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
