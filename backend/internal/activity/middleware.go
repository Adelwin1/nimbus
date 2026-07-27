package activity

import (
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type HandlerOptions struct {
	Action     string
	EntityType string
	Summary    string

	ApplicationIDParam string
	EntityIDParam      string

	Metadata func(
		request *http.Request,
		statusCode int,
	) any
}

func WrapHandler(
	recorder Recorder,
	options HandlerOptions,
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		response := &activityResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next(response, r)

		if response.statusCode < 200 ||
			response.statusCode >= 400 {
			return
		}

		var actorUserID *uuid.UUID

		if value, ok := appmiddleware.GetUserID(
			r.Context(),
		); ok {
			userID := value
			actorUserID = &userID
		}

		applicationID := parseRouteUUID(
			r,
			options.ApplicationIDParam,
		)

		entityID := parseRouteUUID(
			r,
			options.EntityIDParam,
		)

		metadata := any(
			map[string]any{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": response.statusCode,
			},
		)

		if options.Metadata != nil {
			metadata = options.Metadata(
				r,
				response.statusCode,
			)
		}

		RecordRequest(
			recorder,
			r,
			RequestRecordInput{
				ActorUserID:   actorUserID,
				ApplicationID: applicationID,
				Action:        options.Action,
				EntityType:    options.EntityType,
				EntityID:      entityID,
				Summary:       options.Summary,
				Metadata:      metadata,
			},
		)
	}
}

type activityResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *activityResponseWriter) WriteHeader(
	statusCode int,
) {
	if w.wroteHeader {
		return
	}

	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *activityResponseWriter) Write(
	body []byte,
) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(body)
}

func (w *activityResponseWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}

	flusher.Flush()
}

func (w *activityResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func parseRouteUUID(
	request *http.Request,
	parameter string,
) *uuid.UUID {
	if parameter == "" {
		return nil
	}

	value := chi.URLParam(request, parameter)
	if value == "" {
		return nil
	}

	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}

	return &parsed
}
