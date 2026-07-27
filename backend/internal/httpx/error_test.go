package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorReturnsStructuredResponse(
	t *testing.T,
) {
	t.Parallel()

	response := httptest.NewRecorder()

	WriteError(
		response,
		http.StatusBadRequest,
		"invalid_request",
		"The request is invalid.",
		"request-123",
		map[string]any{
			"field": "email",
		},
	)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, expected %d",
			response.Code,
			http.StatusBadRequest,
		)
	}

	var payload ErrorResponse

	if err := json.Unmarshal(
		response.Body.Bytes(),
		&payload,
	); err != nil {
		t.Fatalf(
			"decode response: %v",
			err,
		)
	}

	if payload.Error.Code != "invalid_request" {
		t.Fatalf(
			"code = %q",
			payload.Error.Code,
		)
	}

	if payload.Error.RequestID != "request-123" {
		t.Fatalf(
			"request ID = %q",
			payload.Error.RequestID,
		)
	}
}

func TestWriteErrorMasksInternalMessage(
	t *testing.T,
) {
	t.Parallel()

	response := httptest.NewRecorder()

	WriteError(
		response,
		http.StatusInternalServerError,
		"database_error",
		"postgres password=super-secret",
		"request-456",
		map[string]any{
			"database_url": "secret",
		},
	)

	body := response.Body.String()

	if body == "" {
		t.Fatal("response body was empty")
	}

	if containsAny(
		body,
		"super-secret",
		"database_url",
		"postgres password",
	) {
		t.Fatal(
			"internal error exposed sensitive details",
		)
	}
}

func containsAny(
	value string,
	terms ...string,
) bool {
	for _, term := range terms {
		if len(term) > 0 &&
			contains(value, term) {
			return true
		}
	}

	return false
}

func contains(
	value string,
	term string,
) bool {
	for index := 0; index+len(term) <= len(value); index++ {
		if value[index:index+len(term)] == term {
			return true
		}
	}

	return false
}
