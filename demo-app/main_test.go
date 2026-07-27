package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeploymentFailureAndRollback(
	t *testing.T,
) {
	t.Parallel()

	application := &app{
		token: "test-token",
		logger: slog.New(
			slog.NewTextHandler(
				io.Discard,
				nil,
			),
		),
		state: &demoState{
			Healthy:        true,
			CurrentVersion: "v1.0.0",
			LastAction:     "started",
			UpdatedAt:      time.Now().UTC(),
		},
	}

	server := httptest.NewServer(
		application.routes(),
	)
	defer server.Close()

	assertHealth(
		t,
		server.URL,
		http.StatusOK,
		"v1.0.0",
	)

	postWebhook(
		t,
		server.URL+"/deploy",
		"test-token",
		map[string]any{
			"version": "v2.0.0-broken",
		},
		http.StatusOK,
	)

	assertHealth(
		t,
		server.URL,
		http.StatusServiceUnavailable,
		"v2.0.0-broken",
	)

	postWebhook(
		t,
		server.URL+"/rollback",
		"test-token",
		map[string]any{
			"version": "v1.0.0",
		},
		http.StatusOK,
	)

	assertHealth(
		t,
		server.URL,
		http.StatusOK,
		"v1.0.0",
	)
}

func TestWebhookAuthentication(
	t *testing.T,
) {
	t.Parallel()

	application := &app{
		token: "correct-token",
		logger: slog.New(
			slog.NewTextHandler(
				io.Discard,
				nil,
			),
		),
		state: &demoState{
			Healthy:        true,
			CurrentVersion: "v1.0.0",
			UpdatedAt:      time.Now().UTC(),
		},
	}

	server := httptest.NewServer(
		application.routes(),
	)
	defer server.Close()

	postWebhook(
		t,
		server.URL+"/deploy",
		"wrong-token",
		map[string]any{
			"version": "v2.0.0",
		},
		http.StatusUnauthorized,
	)
}

func assertHealth(
	t *testing.T,
	serverURL string,
	expectedStatus int,
	expectedVersion string,
) {
	t.Helper()

	response, err := http.Get(
		serverURL + "/health",
	)
	if err != nil {
		t.Fatalf(
			"request health endpoint: %v",
			err,
		)
	}
	defer response.Body.Close()

	if response.StatusCode != expectedStatus {
		t.Fatalf(
			"expected status %d, got %d",
			expectedStatus,
			response.StatusCode,
		)
	}

	var payload stateResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(&payload); err != nil {
		t.Fatalf(
			"decode health response: %v",
			err,
		)
	}

	if payload.Version != expectedVersion {
		t.Fatalf(
			"expected version %q, got %q",
			expectedVersion,
			payload.Version,
		)
	}
}

func postWebhook(
	t *testing.T,
	targetURL string,
	token string,
	payload map[string]any,
	expectedStatus int,
) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf(
			"encode webhook payload: %v",
			err,
		)
	}

	request, err := http.NewRequest(
		http.MethodPost,
		targetURL,
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf(
			"create webhook request: %v",
			err,
		)
	}

	request.Header.Set(
		"Authorization",
		"Bearer "+token,
	)
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	response, err :=
		http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf(
			"send webhook request: %v",
			err,
		)
	}
	defer response.Body.Close()

	if response.StatusCode != expectedStatus {
		t.Fatalf(
			"expected status %d, got %d",
			expectedStatus,
			response.StatusCode,
		)
	}
}
