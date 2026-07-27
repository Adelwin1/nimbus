package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type safeResponseRecorder struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *safeResponseRecorder) WriteHeader(
	statusCode int,
) {
	if w.wroteHeader {
		return
	}

	w.statusCode = statusCode
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *safeResponseRecorder) Write(
	body []byte,
) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(body)
}

func (w *safeResponseRecorder) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}

	flusher.Flush()
}

func (w *safeResponseRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func SafeLogging(
	logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				startedAt := time.Now()

				recorder := &safeResponseRecorder{
					ResponseWriter: w,
					statusCode:     http.StatusOK,
				}

				next.ServeHTTP(recorder, r)

				duration := time.Since(startedAt)

				attributes := []any{
					"request_id",
					GetRequestID(r.Context()),
					"method",
					r.Method,
					"route",
					safeRoutePattern(r),
					"status_code",
					recorder.statusCode,
					"duration_ms",
					duration.Milliseconds(),
					"client_ip",
					safeClientIP(r.RemoteAddr),
				}

				switch {
				case recorder.statusCode >= 500:
					logger.Error(
						"http request completed",
						attributes...,
					)

				case recorder.statusCode >= 400:
					logger.Warn(
						"http request completed",
						attributes...,
					)

				default:
					logger.Info(
						"http request completed",
						attributes...,
					)
				}
			},
		)
	}
}

func safeRoutePattern(
	r *http.Request,
) string {
	routeContext := chi.RouteContext(
		r.Context(),
	)

	if routeContext != nil {
		pattern := strings.TrimSpace(
			routeContext.RoutePattern(),
		)

		if pattern != "" {
			return pattern
		}
	}

	return "unmatched"
}

func safeClientIP(
	remoteAddress string,
) string {
	remoteAddress = strings.TrimSpace(
		remoteAddress,
	)

	host, _, err := net.SplitHostPort(
		remoteAddress,
	)
	if err == nil {
		return host
	}

	if parsed := net.ParseIP(
		remoteAddress,
	); parsed != nil {
		return parsed.String()
	}

	return ""
}
