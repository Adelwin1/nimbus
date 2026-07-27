package middleware

import (
	"encoding/json"
	"net/http"
)

const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MB

func RequestBodyLimit(
	maxBytes int64,
) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				if r.Body == nil ||
					r.Body == http.NoBody {
					next.ServeHTTP(w, r)
					return
				}

				if r.ContentLength > maxBytes {
					writePayloadTooLarge(
						w,
						r,
						maxBytes,
					)
					return
				}

				r.Body = http.MaxBytesReader(
					w,
					r.Body,
					maxBytes,
				)

				next.ServeHTTP(w, r)
			},
		)
	}
}

func writePayloadTooLarge(
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
) {
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

	w.WriteHeader(
		http.StatusRequestEntityTooLarge,
	)

	_ = json.NewEncoder(w).Encode(
		map[string]any{
			"error": map[string]any{
				"code": "payload_too_large",
				"message":
					"Request payload is too large.",
				"request_id": GetRequestID(
					r.Context(),
				),
				"details": map[string]any{
					"max_bytes": maxBytes,
				},
			},
		},
	)
}
