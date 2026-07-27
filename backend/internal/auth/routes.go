package auth

import (
	"net/http"

	"github.com/adel/nimbus/backend/internal/activity"
	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func Routes(
	handler *Handler,
	service *Service,
	activityRecorder activity.Recorder,
) http.Handler {
	router := chi.NewRouter()

	router.Post(
		"/register",
		activity.WrapHandler(
			activityRecorder,
			activity.HandlerOptions{
				Action:     activity.ActionUserRegistered,
				EntityType: activity.EntityUser,
				Summary:    "User account was registered.",
			},
			handler.Register,
		),
	)

	router.Post(
		"/login",
		activity.WrapHandler(
			activityRecorder,
			activity.HandlerOptions{
				Action:     activity.ActionUserLoggedIn,
				EntityType: activity.EntityUser,
				Summary:    "User logged in successfully.",
			},
			handler.Login,
		),
	)

	router.Post(
		"/refresh",
		activity.WrapHandler(
			activityRecorder,
			activity.HandlerOptions{
				Action:     activity.ActionUserTokenRefreshed,
				EntityType: activity.EntityUser,
				Summary:    "Authentication token was refreshed.",
			},
			handler.Refresh,
		),
	)

	router.Post(
		"/logout",
		activity.WrapHandler(
			activityRecorder,
			activity.HandlerOptions{
				Action:     activity.ActionUserLoggedOut,
				EntityType: activity.EntityUser,
				Summary:    "User logged out successfully.",
			},
			handler.Logout,
		),
	)

	router.Group(func(protected chi.Router) {
		protected.Use(
			appmiddleware.Authenticate(service),
		)

		protected.Get("/me", handler.Me)
	})

	return router
}
