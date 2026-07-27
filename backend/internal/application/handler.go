package application

import (
	"encoding/json"
	"errors"
	"github.com/adel/nimbus/backend/internal/httpx"
	"io"
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
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

	application, err := h.service.Create(
		r.Context(),
		userID,
		request,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"application": application,
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applications, err := h.service.List(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applications": applications,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	appID, ok := applicationID(w, r)
	if !ok {
		return
	}

	application, err := h.service.Get(
		r.Context(),
		appID,
		userID,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"application": application,
	})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	appID, ok := applicationID(w, r)
	if !ok {
		return
	}

	var request UpdateRequest

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

	application, err := h.service.Update(
		r.Context(),
		appID,
		userID,
		request,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"application": application,
	})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	appID, ok := applicationID(w, r)
	if !ok {
		return
	}

	if err := h.service.Delete(
		r.Context(),
		appID,
		userID,
	); err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	summary, err := h.service.Dashboard(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
	})
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

func applicationID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	appID, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_application_id",
			"Application ID must be a valid UUID.",
		)
		return uuid.Nil, false
	}

	return appID, true
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON object")
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
