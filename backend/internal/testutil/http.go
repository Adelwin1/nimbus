package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type StructuredErrorResponse struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Details   any    `json:"details"`
	} `json:"error"`
}

func JSONRequest(
	t *testing.T,
	method string,
	target string,
	body any,
) *http.Request {
	t.Helper()

	var requestBody io.Reader

	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf(
				"encode request body: %v",
				err,
			)
		}

		requestBody = bytes.NewReader(encoded)
	}

	request := httptest.NewRequest(
		method,
		target,
		requestBody,
	)

	if body != nil {
		request.Header.Set(
			"Content-Type",
			"application/json",
		)
	}

	return request
}

func DecodeJSON[T any](
	t *testing.T,
	response *httptest.ResponseRecorder,
) T {
	t.Helper()

	var payload T

	if err := json.Unmarshal(
		response.Body.Bytes(),
		&payload,
	); err != nil {
		t.Fatalf(
			"decode JSON response: %v\nbody: %s",
			err,
			response.Body.String(),
		)
	}

	return payload
}

func AssertStatus(
	t *testing.T,
	response *httptest.ResponseRecorder,
	expected int,
) {
	t.Helper()

	if response.Code != expected {
		t.Fatalf(
			"status = %d, expected %d\nbody: %s",
			response.Code,
			expected,
			response.Body.String(),
		)
	}
}

func AssertStructuredError(
	t *testing.T,
	response *httptest.ResponseRecorder,
	expectedStatus int,
	expectedCode string,
) StructuredErrorResponse {
	t.Helper()

	AssertStatus(
		t,
		response,
		expectedStatus,
	)

	payload := DecodeJSON[StructuredErrorResponse](
		t,
		response,
	)

	if payload.Error.Code != expectedCode {
		t.Fatalf(
			"error code = %q, expected %q",
			payload.Error.Code,
			expectedCode,
		)
	}

	if payload.Error.Message == "" {
		t.Fatal(
			"structured error message was empty",
		)
	}

	return payload
}
