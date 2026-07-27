package application

import (
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func Routes(
	handler *Handler,
	tokenParser appmiddleware.AccessTokenParser,
) http.Handler {
	router := chi.NewRouter()

	router.Use(appmiddleware.Authenticate(tokenParser))

	router.Post("/", handler.Create)
	router.Get("/", handler.List)
	router.Get("/{appID}", handler.Get)
	router.Patch("/{appID}", handler.Update)
	router.Delete("/{appID}", handler.Delete)

	return router
}
