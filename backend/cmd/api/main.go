package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adel/nimbus/backend/internal/config"
	"github.com/adel/nimbus/backend/internal/database"
	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
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

	router := chi.NewRouter()

	router.Use(appmiddleware.RequestID)
	router.Use(appmiddleware.Logging(logger))
	router.Use(appmiddleware.Recovery(logger))
	router.Use(appmiddleware.CORS(cfg.FrontendOrigin))

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "healthy",
			"service":     "nimbus-api",
			"environment": cfg.AppEnv,
			"request_id":  appmiddleware.GetRequestID(r.Context()),
		})
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info(
			"Nimbus API started",
			"port", cfg.Port,
			"environment", cfg.AppEnv,
		)

		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failure", "error", err)
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
