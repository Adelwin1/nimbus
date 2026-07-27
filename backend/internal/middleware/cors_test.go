package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsConfiguredOrigin(
	t *testing.T,
) {
	t.Parallel()

	handler := CORS(
		"http://localhost:3000",
	)(
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
		http.MethodGet,
		"http://example.com",
		nil,
	)
	request.Header.Set(
		"Origin",
		"http://localhost:3000",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusOK,
		)
	}

	if response.Header().Get(
		"Access-Control-Allow-Origin",
	) != "http://localhost:3000" {
		t.Fatal(
			"allowed origin header was not returned",
		)
	}
}

func TestCORSRejectsUnknownOrigin(
	t *testing.T,
) {
	t.Parallel()

	handler := CORS(
		"https://nimbus.example.com",
	)(
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
		http.MethodGet,
		"http://example.com",
		nil,
	)
	request.Header.Set(
		"Origin",
		"https://evil.example.com",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusForbidden,
		)
	}
}

func TestCORSHandlesPreflight(
	t *testing.T,
) {
	t.Parallel()

	handler := CORS(
		"http://localhost:3000",
	)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				t.Fatal(
					"next handler called during preflight",
				)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodOptions,
		"http://example.com",
		nil,
	)
	request.Header.Set(
		"Origin",
		"http://localhost:3000",
	)
	request.Header.Set(
		"Access-Control-Request-Method",
		http.MethodPost,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusNoContent,
		)
	}
}
