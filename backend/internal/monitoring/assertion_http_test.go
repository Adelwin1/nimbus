package monitoring

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type assertionTransport func(*http.Request) (*http.Response, error)

func (f assertionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func assertionChecker(status int, body string) *Checker {
	c := NewChecker(time.Second)
	c.client.Transport = assertionTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	return c
}

func TestHTTPCheckAppliesRules(t *testing.T) {
	pointer := "/database/connected"
	status := 200
	rules := CheckRules{
		ExpectedStatus: &status,
		JSONPointer:    &pointer,
		JSONExpected:   json.RawMessage(`true`),
	}

	for _, healthy := range []bool{true, false} {
		body := `{"database":{"connected":false},"secret":"never-copy-this"}`
		if healthy {
			body = `{"database":{"connected":true}}`
		}
		result := assertionChecker(200, body).Check(
			context.Background(), "https://example.com/health", 1000, rules,
		)
		if result.Healthy != healthy {
			t.Fatalf("expected healthy=%v: %+v", healthy, result)
		}
		if result.ErrorMessage != nil &&
			strings.Contains(*result.ErrorMessage, "never-copy-this") {
			t.Fatal("response body leaked")
		}
	}

	result := assertionChecker(201, `{}`).Check(
		context.Background(), "https://example.com/health", 1000, rules,
	)
	if result.Healthy || result.ErrorMessage == nil ||
		!strings.Contains(*result.ErrorMessage, "received 201") {
		t.Fatal("wrong status must fail")
	}

	c := assertionChecker(200, strings.Repeat("x", 20))
	c.maxResponseBytes = 8
	result = c.Check(context.Background(), "https://example.com/health", 1000)
	if result.Healthy || result.ErrorMessage == nil ||
		!strings.Contains(*result.ErrorMessage, "size limit") {
		t.Fatal("oversized body must fail")
	}
}
