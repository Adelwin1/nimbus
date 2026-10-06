package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/adel/nimbus/backend/internal/apperrors"
	"github.com/adel/nimbus/backend/internal/githubapp"
	"github.com/adel/nimbus/backend/internal/journeys"
	"github.com/adel/nimbus/backend/internal/repairs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adel/nimbus/backend/internal/activity"
	"github.com/adel/nimbus/backend/internal/alerts"
	"github.com/adel/nimbus/backend/internal/application"
	"github.com/adel/nimbus/backend/internal/auth"
	"github.com/adel/nimbus/backend/internal/config"
	appcrypto "github.com/adel/nimbus/backend/internal/crypto"
	"github.com/adel/nimbus/backend/internal/database"
	"github.com/adel/nimbus/backend/internal/deployment"
	"github.com/adel/nimbus/backend/internal/incident"
	"github.com/adel/nimbus/backend/internal/insights"
	"github.com/adel/nimbus/backend/internal/live"
	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/adel/nimbus/backend/internal/monitoring"
	"github.com/adel/nimbus/backend/internal/projects"
	"github.com/adel/nimbus/backend/internal/publicstatus"
	"github.com/go-chi/chi/v5"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	appContext, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	db, err := database.Open(appContext, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Authentication dependencies.
	authRepository := auth.NewRepository(db)

	authService := auth.NewService(
		authRepository,
		cfg.JWTAccessSecret,
		cfg.JWTRefreshSecret,
		cfg.AccessTokenTTL,
		cfg.RefreshTokenTTL,
	)

	authHandler := auth.NewHandler(authService)

	// Application-management dependencies.
	encryptor, err := appcrypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		logger.Error("configure webhook encryption", "error", err)
		os.Exit(1)
	}

	applicationRepository := application.NewRepository(db)
	applicationService := application.NewService(
		applicationRepository,
		encryptor,
	)
	applicationHandler := application.NewHandler(applicationService)

	// Real application monitoring dependencies.
	monitoringRepository := monitoring.NewRepository(db)
	healthChecker := monitoring.NewChecker(cfg.HTTPCheckTimeout)
	monitoringService := monitoring.NewService(
		monitoringRepository,
		healthChecker,
	)
	monitoringHandler := monitoring.NewHandler(monitoringService)
	monitoringScheduler := monitoring.NewScheduler(
		monitoringService,
		logger,
		cfg.MonitorWorkerInterval,
	)

	// Deployment execution and verification dependencies.
	deploymentRepository := deployment.NewRepository(db)
	webhookExecutor := deployment.NewWebhookExecutor(
		encryptor,
		cfg.HTTPCheckTimeout,
	)
	activityRepository := activity.NewRepository(
		db,
		logger,
	)
	activityRecorder := activity.NewBufferedRecorder(
		appContext,
		activityRepository,
		logger,
		512,
	)

	deploymentService := deployment.NewService(
		appContext,
		deploymentRepository,
		webhookExecutor,
		monitoringService,
		activityRecorder,
	)

	deploymentHandler := deployment.NewHandler(
		deploymentService,
		activityRecorder,
	)

	liveHandler := live.NewHandler(db)

	incidentRepository := incident.NewRepository(
		db,
		activityRecorder,
	)
	incidentService := incident.NewService(
		incidentRepository,
	)

	rollbackService := incident.NewRollbackService(
		appContext,
		incidentRepository,
		deploymentRepository,
		webhookExecutor,
		monitoringService,
		activityRecorder,
	)

	incidentHandler := incident.NewHandler(
		incidentService,
		rollbackService,
		activityRecorder,
	)

	incidentDetector := incident.NewDetector(
		db,
		incidentService,
		logger,
	)

	router := chi.NewRouter()
	router.Use(appmiddleware.SecurityHeaders)
	rateLimiter := appmiddleware.NewDefaultRateLimiter()
	router.Use(rateLimiter.Middleware)
	router.Use(appmiddleware.CORSFromEnvironment())

	router.Use(appmiddleware.RequestID)
	router.Use(appmiddleware.SafeLogging(logger))
	router.Use(appmiddleware.Recovery(logger))

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "healthy",
			"service":     "nimbus-api",
			"environment": cfg.AppEnv,
			"request_id":  appmiddleware.GetRequestID(r.Context()),
		})
	})

	router.Mount(
		"/api/v1/auth",
		auth.Routes(
			authHandler,
			authService,
			activityRecorder,
		),
	)

	githubHandler := githubapp.New(db, encryptor)
	router.Get("/api/v1/github/authorize", githubHandler.Authorize)
	router.Get("/api/v1/github/callback", githubHandler.Callback)
	router.Route("/api/v1/github", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/status", githubHandler.Status)
		p.Post("/start", githubHandler.Start)
		p.Delete("/connection", githubHandler.Disconnect)
		p.Get("/repositories", githubHandler.Repositories)
		p.Put("/apps/{appID}/repository", githubHandler.Link)
	})

	ps := &publicstatus.Handler{DB: db}
	router.Get("/api/v1/status/{slug}", ps.Public)

	projectsHandler := &projects.Handler{DB: db}
	router.Route("/api/v1/projects", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/", projectsHandler.List)
		p.Put("/apps/{appID}", projectsHandler.Save)
	})

	alertHandler := &alerts.Handler{DB: db}
	router.Route("/api/v1/alerts", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/", alertHandler.Inbox)
		p.Get("/rules", alertHandler.Rules)
		p.Put("/rules/{appID}", alertHandler.Rules)
	})

	journeyHandler := &journeys.Handler{DB: db, GitHub: githubHandler}
	router.Route("/api/v1/journeys", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/apps/{appID}", journeyHandler.Applications)
		p.Post("/apps/{appID}", journeyHandler.Applications)
		p.Get("/{journeyID}/runs", journeyHandler.Runs)
		p.Post("/{journeyID}/runs", journeyHandler.Runs)
	})

	errorHandler := &apperrors.Handler{DB: db}
	router.Post("/api/v1/error-reports/{appID}", errorHandler.Report)
	router.Route("/api/v1/application-errors", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/apps/{appID}", errorHandler.List)
		p.Post("/apps/{appID}/token", errorHandler.Token)
	})

	repairHandler := &repairs.Handler{DB: db}
	router.Route("/api/v1/repair-reviews", func(p chi.Router) {
		p.Use(appmiddleware.Authenticate(authService))
		p.Get("/apps/{appID}", repairHandler.Reviews)
		p.Post("/apps/{appID}", repairHandler.Reviews)
		p.Post("/{reviewID}/decision", repairHandler.Decide)
	})

	router.Route("/api/v1/apps", func(protected chi.Router) {
		protected.Use(appmiddleware.Authenticate(authService))

		protected.Get("/{appID}/publication", ps.Settings)
		protected.Put("/{appID}/publication", ps.Settings)
		protected.Post("/", applicationHandler.Create)
		protected.Get("/", applicationHandler.List)
		protected.Get("/{appID}", applicationHandler.Get)
		protected.Get("/{appID}/check-rules", monitoringHandler.CheckRulesSettings)
		protected.Put("/{appID}/check-rules", monitoringHandler.CheckRulesSettings)
		protected.Patch("/{appID}", applicationHandler.Update)
		protected.Delete("/{appID}", applicationHandler.Delete)

		protected.Get(
			"/{appID}/health",
			monitoringHandler.Overview,
		)
		protected.Get(
			"/{appID}/health/history",
			monitoringHandler.History,
		)
		protected.Post(
			"/{appID}/health/check",
			activity.WrapHandler(
				activityRecorder,
				activity.HandlerOptions{
					Action:             activity.ActionHealthCheckRequested,
					EntityType:         activity.EntityApplication,
					Summary:            "Manual health check was requested.",
					ApplicationIDParam: "appID",
					EntityIDParam:      "appID",
				},
				monitoringHandler.CheckNow,
			),
		)

		protected.Post(
			"/{appID}/deployments",
			deploymentHandler.Create,
		)
		protected.Get(
			"/{appID}/deployments",
			deploymentHandler.List,
		)

		protected.Get(
			"/{appID}/events",
			liveHandler.StreamApplication,
		)

		protected.Get(
			"/{appID}/incidents",
			incidentHandler.ListByApplication,
		)
	})

	router.Group(func(protected chi.Router) {
		protected.Use(appmiddleware.Authenticate(authService))
		protected.Get("/api/v1/dashboard/analytics", insights.Handler{Store: &insights.Repository{DB: db}}.Overview)
		protected.Get(
			"/api/v1/dashboard",
			applicationHandler.Dashboard,
		)
	})

	router.Mount(
		"/api/v1/deployments",
		deployment.DetailRoutes(
			deploymentHandler,
			authService,
		),
	)

	router.Mount(
		"/api/v1/incidents",
		incident.Routes(
			incidentHandler,
			authService,
		),
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go monitoringScheduler.Run(appContext)
	go incidentDetector.Run(appContext)

	if err := deploymentService.ResumeUnfinished(
		appContext,
	); err != nil {
		logger.Error(
			"resume unfinished deployments",
			"error",
			err,
		)
	}

	if err := rollbackService.ResumeUnfinished(
		appContext,
	); err != nil {
		logger.Error(
			"resume unfinished rollbacks",
			"error",
			err,
		)
	}

	go func() {
		logger.Info(
			"Nimbus API started",
			"port", cfg.Port,
			"environment", cfg.AppEnv,
		)

		serverErrors <- server.ListenAndServe()
	}()

	select {
	case serverError := <-serverErrors:
		if !errors.Is(serverError, http.ErrServerClosed) {
			logger.Error("server failure", "error", serverError)
			os.Exit(1)
		}

	case <-appContext.Done():
		logger.Info("shutdown signal received")
	}

	shutdownContext, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Nimbus API stopped")
}

func writeJSON(
	w http.ResponseWriter,
	statusCode int,
	payload any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if payload == nil {
		return
	}

	_ = json.NewEncoder(w).Encode(payload)
}
