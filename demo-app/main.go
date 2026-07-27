package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxRequestBytes = 64 * 1024

type demoState struct {
	mu sync.RWMutex

	Healthy         bool
	CurrentVersion  string
	PreviousVersion string
	LastAction      string
	UpdatedAt       time.Time
}

type stateResponse struct {
	Status          string    `json:"status"`
	Service         string    `json:"service"`
	Version         string    `json:"version"`
	PreviousVersion string    `json:"previous_version,omitempty"`
	LastAction      string    `json:"last_action"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type app struct {
	token  string
	logger *slog.Logger
	state  *demoState
}

func main() {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				Level: slog.LevelInfo,
			},
		),
	)

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}

	token := strings.TrimSpace(
		os.Getenv("DEMO_WEBHOOK_TOKEN"),
	)
	if token == "" {
		logger.Error(
			"DEMO_WEBHOOK_TOKEN is required",
		)
		os.Exit(1)
	}

	initialVersion := strings.TrimSpace(
		os.Getenv("DEMO_INITIAL_VERSION"),
	)
	if initialVersion == "" {
		initialVersion = "v1.0.0"
	}

	application := &app{
		token:  token,
		logger: logger,
		state: &demoState{
			Healthy:        true,
			CurrentVersion: initialVersion,
			LastAction:     "started",
			UpdatedAt:      time.Now().UTC(),
		},
	}

	server := &http.Server{
		Addr:              "0.0.0.0:" + port,
		Handler:           application.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(
		chan error,
		1,
	)

	go func() {
		logger.Info(
			"Nimbus demo application started",
			"port",
			port,
			"version",
			initialVersion,
		)

		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignals := make(
		chan os.Signal,
		1,
	)

	signal.Notify(
		shutdownSignals,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	select {
	case signalValue := <-shutdownSignals:
		logger.Info(
			"shutdown signal received",
			"signal",
			signalValue.String(),
		)

	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error(
				"server failed",
				"error",
				err.Error(),
			)
			os.Exit(1)
		}
	}

	shutdownContext, cancel :=
		context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
	defer cancel()

	if err := server.Shutdown(
		shutdownContext,
	); err != nil {
		logger.Error(
			"graceful shutdown failed",
			"error",
			err.Error(),
		)
		os.Exit(1)
	}
}

func (a *app) routes() http.Handler {
	router := http.NewServeMux()

	router.HandleFunc(
		"GET /",
		a.handleRoot,
	)
	router.HandleFunc(
		"GET /health",
		a.handleHealth,
	)
	router.HandleFunc(
		"POST /deploy",
		a.requireWebhookToken(
			a.handleDeploy,
		),
	)
	router.HandleFunc(
		"POST /rollback",
		a.requireWebhookToken(
			a.handleRollback,
		),
	)

	return securityHeaders(router)
}

func (a *app) handleRoot(
	w http.ResponseWriter,
	_ *http.Request,
) {
	snapshot := a.snapshot()

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"service": "nimbus-demo-application",
			"message": "A real application monitored and recovered by Nimbus.",
			"state":   snapshot,
			"endpoints": []string{
				"GET /health",
				"POST /deploy",
				"POST /rollback",
			},
		},
	)
}

func (a *app) handleHealth(
	w http.ResponseWriter,
	_ *http.Request,
) {
	snapshot := a.snapshot()

	statusCode := http.StatusOK
	if snapshot.Status != "healthy" {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(
		w,
		statusCode,
		snapshot,
	)
}

func (a *app) handleDeploy(
	w http.ResponseWriter,
	r *http.Request,
) {
	payload, err := decodePayload(w, r)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": err.Error(),
			},
		)
		return
	}

	version := extractVersion(payload)
	if version == "" {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "deployment version is required",
			},
		)
		return
	}

	healthy := !isFailureVersion(version)

	if explicitHealthy, exists :=
		findBool(payload, "healthy"); exists {
		healthy = explicitHealthy
	}

	if forceFailure, exists :=
		findBool(
			payload,
			"fail",
			"unhealthy",
			"force_failure",
			"force_unhealthy",
		); exists && forceFailure {
		healthy = false
	}

	a.state.mu.Lock()

	if version != a.state.CurrentVersion {
		a.state.PreviousVersion =
			a.state.CurrentVersion
	}

	a.state.CurrentVersion = version
	a.state.Healthy = healthy
	a.state.LastAction = "deploy"
	a.state.UpdatedAt = time.Now().UTC()

	a.state.mu.Unlock()

	a.logger.Info(
		"deployment webhook applied",
		"version",
		version,
		"healthy",
		healthy,
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"accepted": true,
			"action":   "deploy",
			"state":    a.snapshot(),
		},
	)
}

func (a *app) handleRollback(
	w http.ResponseWriter,
	r *http.Request,
) {
	payload, err := decodePayload(w, r)
	if err != nil {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": err.Error(),
			},
		)
		return
	}

	targetVersion := extractVersion(payload)

	a.state.mu.Lock()

	if targetVersion == "" {
		targetVersion =
			a.state.PreviousVersion
	}

	if targetVersion == "" {
		a.state.mu.Unlock()

		writeJSON(
			w,
			http.StatusConflict,
			map[string]string{
				"error": "no rollback version is available",
			},
		)
		return
	}

	failedVersion := a.state.CurrentVersion

	a.state.CurrentVersion = targetVersion
	a.state.PreviousVersion = failedVersion
	a.state.Healthy = true
	a.state.LastAction = "rollback"
	a.state.UpdatedAt = time.Now().UTC()

	a.state.mu.Unlock()

	a.logger.Info(
		"rollback webhook applied",
		"failed_version",
		failedVersion,
		"restored_version",
		targetVersion,
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"accepted":         true,
			"action":           "rollback",
			"restored_version": targetVersion,
			"state":            a.snapshot(),
		},
	)
}

func (a *app) requireWebhookToken(
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		providedToken := bearerToken(
			r.Header.Get("Authorization"),
		)

		if providedToken == "" {
			providedToken = strings.TrimSpace(
				r.Header.Get("X-Webhook-Token"),
			)
		}

		if providedToken == "" {
			providedToken = strings.TrimSpace(
				r.Header.Get("X-Nimbus-Token"),
			)
		}

		if providedToken != a.token {
			writeJSON(
				w,
				http.StatusUnauthorized,
				map[string]string{
					"error": "invalid webhook token",
				},
			)
			return
		}

		next(w, r)
	}
}

func (a *app) snapshot() stateResponse {
	a.state.mu.RLock()
	defer a.state.mu.RUnlock()

	status := "healthy"
	if !a.state.Healthy {
		status = "unhealthy"
	}

	return stateResponse{
		Status:          status,
		Service:         "nimbus-demo-application",
		Version:         a.state.CurrentVersion,
		PreviousVersion: a.state.PreviousVersion,
		LastAction:      a.state.LastAction,
		UpdatedAt:       a.state.UpdatedAt,
	}
}

func decodePayload(
	w http.ResponseWriter,
	r *http.Request,
) (map[string]any, error) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxRequestBytes,
	)

	defer r.Body.Close()

	payload := make(map[string]any)

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf(
			"invalid JSON payload",
		)
	}

	return payload, nil
}

func extractVersion(
	payload map[string]any,
) string {
	versionKeys := map[string]struct{}{
		"version":          {},
		"target_version":   {},
		"rollback_version": {},
		"release_version":  {},
	}

	return findStringRecursive(
		payload,
		versionKeys,
	)
}

func findStringRecursive(
	value any,
	keys map[string]struct{},
) string {
	switch typedValue := value.(type) {
	case map[string]any:
		for key, child := range typedValue {
			normalizedKey := strings.ToLower(
				strings.TrimSpace(key),
			)

			if _, exists := keys[normalizedKey]; exists {
				if stringValue, ok :=
					child.(string); ok {
					return strings.TrimSpace(
						stringValue,
					)
				}
			}
		}

		for _, child := range typedValue {
			if result := findStringRecursive(
				child,
				keys,
			); result != "" {
				return result
			}
		}

	case []any:
		for _, child := range typedValue {
			if result := findStringRecursive(
				child,
				keys,
			); result != "" {
				return result
			}
		}
	}

	return ""
}

func findBool(
	payload map[string]any,
	keys ...string,
) (bool, bool) {
	searchKeys := make(
		map[string]struct{},
		len(keys),
	)

	for _, key := range keys {
		searchKeys[strings.ToLower(key)] =
			struct{}{}
	}

	return findBoolRecursive(
		payload,
		searchKeys,
	)
}

func findBoolRecursive(
	value any,
	keys map[string]struct{},
) (bool, bool) {
	switch typedValue := value.(type) {
	case map[string]any:
		for key, child := range typedValue {
			normalizedKey := strings.ToLower(
				strings.TrimSpace(key),
			)

			if _, exists := keys[normalizedKey]; exists {
				if booleanValue, ok :=
					child.(bool); ok {
					return booleanValue, true
				}
			}
		}

		for _, child := range typedValue {
			if result, exists :=
				findBoolRecursive(
					child,
					keys,
				); exists {
				return result, true
			}
		}

	case []any:
		for _, child := range typedValue {
			if result, exists :=
				findBoolRecursive(
					child,
					keys,
				); exists {
				return result, true
			}
		}
	}

	return false, false
}

func isFailureVersion(
	version string,
) bool {
	lowerVersion := strings.ToLower(version)

	return strings.Contains(
		lowerVersion,
		"broken",
	) || strings.Contains(
		lowerVersion,
		"fail",
	)
}

func bearerToken(
	value string,
) string {
	const prefix = "Bearer "

	if !strings.HasPrefix(value, prefix) {
		return ""
	}

	return strings.TrimSpace(
		strings.TrimPrefix(value, prefix),
	)
}

func securityHeaders(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			w.Header().Set(
				"X-Content-Type-Options",
				"nosniff",
			)
			w.Header().Set(
				"X-Frame-Options",
				"DENY",
			)
			w.Header().Set(
				"Cache-Control",
				"no-store",
			)

			next.ServeHTTP(w, r)
		},
	)
}

func writeJSON(
	w http.ResponseWriter,
	statusCode int,
	payload any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(payload)
}
