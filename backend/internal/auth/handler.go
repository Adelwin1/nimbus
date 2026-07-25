package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request RegisterRequest

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

	response, err := h.service.Register(r.Context(), request)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest

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

	response, err := h.service.Login(r.Context(), request)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var request RefreshRequest

	if err := decodeJSON(r, &request); err != nil ||
		request.RefreshToken == "" {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"A refresh token is required.",
		)
		return
	}

	response, err := h.service.Refresh(
		r.Context(),
		request.RefreshToken,
	)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var request LogoutRequest

	if err := decodeJSON(r, &request); err != nil ||
		request.RefreshToken == "" {
		writeError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"A refresh token is required.",
		)
		return
	}

	if err := h.service.Logout(
		r.Context(),
		request.RefreshToken,
	); err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
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

	user, err := h.service.GetUser(r.Context(), userID)
	if err != nil {
		h.handleServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user": user,
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

	case errors.Is(err, ErrEmailTaken):
		writeError(
			w,
			r,
			http.StatusConflict,
			"email_taken",
			"An account with that email already exists.",
		)

	case errors.Is(err, ErrInvalidCredentials):
		writeError(
			w,
			r,
			http.StatusUnauthorized,
			"invalid_credentials",
			"Invalid email or password.",
		)

	case errors.Is(err, ErrInvalidToken),
		errors.Is(err, ErrExpiredToken),
		errors.Is(err, ErrRevokedSession):
		writeError(
			w,
			r,
			http.StatusUnauthorized,
			"invalid_token",
			"The authentication token is invalid or expired.",
		)

	case errors.Is(err, ErrUserNotFound):
		writeError(
			w,
			r,
			http.StatusNotFound,
			"user_not_found",
			"User not found.",
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

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(destination)
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
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": appmiddleware.GetRequestID(r.Context()),
		},
	})
}
