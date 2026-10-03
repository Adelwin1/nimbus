package incident

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

	router.Use(
		appmiddleware.Authenticate(tokenParser),
	)

	router.Get("/", handler.List)

	router.Get(
		"/{incidentID}",
		handler.Detail,
	)

	router.Post(
		"/{incidentID}/acknowledge",
		handler.Acknowledge,
	)

	router.Post(
		"/{incidentID}/resolve",
		handler.Resolve,
	)

	router.Post(
		"/{incidentID}/rollback",
		handler.Rollback,
	)

	router.Post("/{incidentID}/notes", handler.AddNote)

	return router
}
