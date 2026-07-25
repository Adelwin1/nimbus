package middleware

import (
	"net/http"

	chicors "github.com/go-chi/cors"
)

func CORS(frontendOrigin string) func(http.Handler) http.Handler {
	return chicors.Handler(chicors.Options{
		AllowedOrigins: []string{frontendOrigin},
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Request-ID",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
		},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
