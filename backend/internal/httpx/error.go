package httpx

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}

func WriteError(
	w http.ResponseWriter,
	statusCode int,
	code string,
	message string,
	requestID string,
	details any,
) {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	requestID = strings.TrimSpace(requestID)

	if code == "" {
		code = "request_failed"
	}

	if message == "" {
		message = http.StatusText(statusCode)
	}

	if statusCode >= http.StatusInternalServerError {
		message = "An internal server error occurred."
		details = nil
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)
	w.Header().Set(
		"Cache-Control",
		"no-store",
	)
	w.Header().Set(
		"X-Content-Type-Options",
		"nosniff",
	)

	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(
		ErrorResponse{
			Error: ErrorDetail{
				Code:      code,
				Message:   message,
				RequestID: requestID,
				Details:   details,
			},
		},
	)
}

func BadRequest(
	w http.ResponseWriter,
	requestID string,
	code string,
	message string,
	details any,
) {
	WriteError(
		w,
		http.StatusBadRequest,
		code,
		message,
		requestID,
		details,
	)
}

func Unauthorized(
	w http.ResponseWriter,
	requestID string,
	code string,
	message string,
) {
	WriteError(
		w,
		http.StatusUnauthorized,
		code,
		message,
		requestID,
		nil,
	)
}

func Forbidden(
	w http.ResponseWriter,
	requestID string,
	code string,
	message string,
) {
	WriteError(
		w,
		http.StatusForbidden,
		code,
		message,
		requestID,
		nil,
	)
}

func NotFound(
	w http.ResponseWriter,
	requestID string,
	code string,
	message string,
) {
	WriteError(
		w,
		http.StatusNotFound,
		code,
		message,
		requestID,
		nil,
	)
}

func Conflict(
	w http.ResponseWriter,
	requestID string,
	code string,
	message string,
) {
	WriteError(
		w,
		http.StatusConflict,
		code,
		message,
		requestID,
		nil,
	)
}

func Internal(
	w http.ResponseWriter,
	requestID string,
) {
	WriteError(
		w,
		http.StatusInternalServerError,
		"internal_error",
		"An internal server error occurred.",
		requestID,
		nil,
	)
}
