package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestSafeLoggingDoesNotLogSensitiveData(
	t *testing.T,
) {
	t.Parallel()

	var output bytes.Buffer

	logger := slog.New(
		slog.NewJSONHandler(
			&output,
			nil,
		),
	)

	handler := SafeLogging(logger)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com/login?token=secret-token",
		strings.NewReader(
			`{"password":"super-secret"}`,
		),
	)

	request.Header.Set(
		"Authorization",
		"Bearer jwt-secret",
	)
	request.Header.Set(
		"Cookie",
		"session=session-secret",
	)

	routeContext := chi.NewRouteContext()
	routeContext.RoutePatterns = []string{
		"/api/v1/auth/login",
	}

	request = request.WithContext(
		context.WithValue(
			request.Context(),
			chi.RouteCtxKey,
			routeContext,
		),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	logged := output.String()

	for _, secret := range []string{
		"secret-token",
		"super-secret",
		"jwt-secret",
		"session-secret",
		"Authorization",
		"Cookie",
	} {
		if strings.Contains(logged, secret) {
			t.Fatalf(
				"log exposed sensitive value %q",
				secret,
			)
		}
	}

	if !strings.Contains(
		logged,
		"/api/v1/auth/login",
	) {
		t.Fatal(
			"safe route pattern was not logged",
		)
	}
}

func TestSafeClientIP(
	t *testing.T,
) {
	t.Parallel()

	actual := safeClientIP(
		"203.0.113.20:4567",
	)

	if actual != "203.0.113.20" {
		t.Fatalf(
			"IP = %q",
			actual,
		)
	}
}
