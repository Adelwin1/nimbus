package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(
	t *testing.T,
) {
	t.Parallel()

	handler := SecurityHeaders(
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

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	expected := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Permissions-Policy":           "camera=(), microphone=(), geolocation=()",
		"Cross-Origin-Resource-Policy": "same-site",
	}

	for header, expectedValue := range expected {
		actual := response.Header().Get(header)

		if actual != expectedValue {
			t.Fatalf(
				"%s = %q, expected %q",
				header,
				actual,
				expectedValue,
			)
		}
	}
}
