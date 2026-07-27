package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const defaultAllowedOrigins = "http://localhost:3000,http://127.0.0.1:3000"

func CORS(
	allowedOriginsValue string,
) func(http.Handler) http.Handler {
	allowedOrigins := parseAllowedOrigins(
		allowedOriginsValue,
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				origin := strings.TrimSpace(
					r.Header.Get("Origin"),
				)

				addVaryHeader(w, "Origin")
				addVaryHeader(
					w,
					"Access-Control-Request-Method",
				)
				addVaryHeader(
					w,
					"Access-Control-Request-Headers",
				)

				// Requests without an Origin header are not
				// browser cross-origin requests.
				if origin == "" {
					next.ServeHTTP(w, r)
					return
				}

				if _, allowed := allowedOrigins[origin]; !allowed {
					writeCORSForbidden(w, r)
					return
				}

				w.Header().Set(
					"Access-Control-Allow-Origin",
					origin,
				)
				w.Header().Set(
					"Access-Control-Allow-Methods",
					"GET, POST, PATCH, DELETE, OPTIONS",
				)
				w.Header().Set(
					"Access-Control-Allow-Headers",
					"Accept, Authorization, Content-Type, X-Request-ID",
				)
				w.Header().Set(
					"Access-Control-Expose-Headers",
					"X-Request-ID, Retry-After, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset",
				)
				w.Header().Set(
					"Access-Control-Max-Age",
					"600",
				)

				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}

				next.ServeHTTP(w, r)
			},
		)
	}
}

func CORSFromEnvironment() func(
	http.Handler,
) http.Handler {
	return CORS(
		os.Getenv("CORS_ALLOWED_ORIGINS"),
	)
}

func parseAllowedOrigins(
	value string,
) map[string]struct{} {
	if strings.TrimSpace(value) == "" {
		value = defaultAllowedOrigins
	}

	allowed := make(map[string]struct{})

	for _, rawOrigin := range strings.Split(
		value,
		",",
	) {
		origin := strings.TrimSpace(rawOrigin)
		if origin == "" {
			continue
		}

		parsed, err := url.Parse(origin)
		if err != nil {
			continue
		}

		if parsed.Scheme != "http" &&
			parsed.Scheme != "https" {
			continue
		}

		if parsed.Host == "" {
			continue
		}

		if parsed.Path != "" &&
			parsed.Path != "/" {
			continue
		}

		normalized := parsed.Scheme +
			"://" +
			parsed.Host

		allowed[normalized] = struct{}{}
	}

	return allowed
}

func addVaryHeader(
	w http.ResponseWriter,
	value string,
) {
	existing := w.Header().Values("Vary")

	for _, header := range existing {
		for _, item := range strings.Split(
			header,
			",",
		) {
			if strings.EqualFold(
				strings.TrimSpace(item),
				value,
			) {
				return
			}
		}
	}

	w.Header().Add("Vary", value)
}

func writeCORSForbidden(
	w http.ResponseWriter,
	r *http.Request,
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

	w.WriteHeader(http.StatusForbidden)

	_ = json.NewEncoder(w).Encode(
		map[string]any{
			"error": map[string]any{
				"code":    "origin_not_allowed",
				"message": "The request origin is not allowed.",
				"request_id": GetRequestID(
					r.Context(),
				),
			},
		},
	)
}
