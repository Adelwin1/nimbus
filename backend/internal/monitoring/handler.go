package monitoring

import (
	"encoding/json"
	"errors"
	"github.com/adel/nimbus/backend/internal/httpx"
	"io"
	"net/http"
	"strconv"

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

func (h *Handler) CheckNow(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := applicationID(w, r)
	if !ok {
		return
	}

	check, state, err := h.service.CheckNow(
		r.Context(),
		applicationID,
		userID,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"check": check,
		"state": state,
	})
}

func (h *Handler) Overview(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := applicationID(w, r)
	if !ok {
		return
	}

	overview, err := h.service.GetOverview(
		r.Context(),
		applicationID,
		userID,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"health": overview,
	})
}

func (h *Handler) History(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	applicationID, ok := applicationID(w, r)
	if !ok {
		return
	}

	limit, err := parseIntegerQuery(r, "limit", 25)
	if err != nil || limit < 1 || limit > 100 {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_limit",
			"Limit must be between 1 and 100.",
		)
		return
	}

	offset, err := parseIntegerQuery(r, "offset", 0)
	if err != nil || offset < 0 {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_offset",
			"Offset must be zero or greater.",
		)
		return
	}

	history, err := h.service.GetHistory(
		r.Context(),
		applicationID,
		userID,
		limit,
		offset,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (h *Handler) handleServiceError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, ErrApplicationNotFound):
		writeError(
			w,
			r,
			http.StatusNotFound,
			"application_not_found",
			"Application not found.",
		)

	case errors.Is(err, ErrCheckInProgress):
		writeError(
			w,
			r,
			http.StatusConflict,
			"check_in_progress",
			"A health check is already running for this application.",
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

func applicationID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	value := chi.URLParam(r, "appID")

	applicationID, err := uuid.Parse(value)
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

	return applicationID, true
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

func parseIntegerQuery(
	r *http.Request,
	name string,
	defaultValue int,
) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return defaultValue, nil
	}

	return strconv.Atoi(value)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain one JSON object")
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
