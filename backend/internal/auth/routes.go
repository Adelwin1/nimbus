package auth

import (
	"net/http"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func Routes(
	handler *Handler,
	service *Service,
) http.Handler {
	router := chi.NewRouter()

	router.Post("/register", handler.Register)
	router.Post("/login", handler.Login)
	router.Post("/refresh", handler.Refresh)
	router.Post("/logout", handler.Logout)

	router.Group(func(protected chi.Router) {
		protected.Use(appmiddleware.Authenticate(service))
		protected.Get("/me", handler.Me)
	})

	return router
}
