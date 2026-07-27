package deployment

import (
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func DetailRoutes(
	handler *Handler,
	tokenParser appmiddleware.AccessTokenParser,
) http.Handler {
	router := chi.NewRouter()

	router.Use(appmiddleware.Authenticate(tokenParser))
	router.Get("/{deploymentID}", handler.Detail)

	return router
}
