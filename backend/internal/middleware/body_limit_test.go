package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestBodyLimitRejectsDeclaredOversizedBody(
	t *testing.T,
) {
	t.Parallel()

	nextCalled := false

	handler := RequestBodyLimit(5)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com",
		strings.NewReader("123456"),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if nextCalled {
		t.Fatal(
			"next handler was called for oversized body",
		)
	}

	if response.Code !=
		http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusRequestEntityTooLarge,
		)
	}

	if !strings.Contains(
		response.Body.String(),
		"payload_too_large",
	) {
		t.Fatal(
			"structured payload error was not returned",
		)
	}
}

func TestRequestBodyLimitAllowsValidBody(
	t *testing.T,
) {
	t.Parallel()

	handler := RequestBodyLimit(10)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf(
						"read request body: %v",
						err,
					)
				}

				if string(body) != "hello" {
					t.Fatalf(
						"body = %q",
						string(body),
					)
				}

				w.WriteHeader(http.StatusCreated)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com",
		strings.NewReader("hello"),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusCreated,
		)
	}
}

func TestRequestBodyLimitRestrictsStreamingBody(
	t *testing.T,
) {
	t.Parallel()

	var readError error

	handler := RequestBodyLimit(5)(
		http.HandlerFunc(
			func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				_, readError = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			},
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"http://example.com",
		io.NopCloser(
			strings.NewReader("123456"),
		),
	)

	request.ContentLength = -1

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if readError == nil {
		t.Fatal(
			"expected oversized streaming body error",
		)
	}
}
