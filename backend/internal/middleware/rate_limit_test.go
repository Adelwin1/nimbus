package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiterAllowsRequestsWithinLimit(
	t *testing.T,
) {
	t.Parallel()

	limiter := NewRateLimiter(
		2,
		time.Minute,
	)

	handler := limiter.Middleware(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(
			http.MethodGet,
			"http://example.com",
			nil,
		)
		request.RemoteAddr =
			"203.0.113.10:12345"

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf(
				"attempt %d status = %d",
				attempt+1,
				response.Code,
			)
		}
	}
}

func TestRateLimiterRejectsRequestsOverLimit(
	t *testing.T,
) {
	t.Parallel()

	limiter := NewRateLimiter(
		1,
		time.Minute,
	)

	handler := limiter.Middleware(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	firstRequest := httptest.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)
	firstRequest.RemoteAddr =
		"203.0.113.20:12345"

	firstResponse := httptest.NewRecorder()

	handler.ServeHTTP(
		firstResponse,
		firstRequest,
	)

	secondRequest := httptest.NewRequest(
		http.MethodGet,
		"http://example.com",
		nil,
	)
	secondRequest.RemoteAddr =
		"203.0.113.20:54321"

	secondResponse := httptest.NewRecorder()

	handler.ServeHTTP(
		secondResponse,
		secondRequest,
	)

	if secondResponse.Code !=
		http.StatusTooManyRequests {
		t.Fatalf(
			"status = %d, expected %d",
			secondResponse.Code,
			http.StatusTooManyRequests,
		)
	}

	if !strings.Contains(
		secondResponse.Body.String(),
		"rate_limit_exceeded",
	) {
		t.Fatal(
			"structured rate-limit error missing",
		)
	}

	if secondResponse.Header().Get(
		"Retry-After",
	) == "" {
		t.Fatal("Retry-After header missing")
	}
}
