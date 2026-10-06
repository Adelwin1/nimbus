package githubapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfiguredRequiresHTTPS(t *testing.T) {
	h := &Handler{ClientID: "id", ClientSecret: "secret", AppID: "42", CallbackURL: "https://api.example.com/api/v1/github/callback", FrontendURL: "https://example.com"}
	if !h.configured() {
		t.Fatal("valid config rejected")
	}
	h.CallbackURL = "http://api.example.com/callback"
	if h.configured() {
		t.Fatal("insecure callback accepted")
	}
}
func TestCallbackRejectsMissingBrowserBinding(t *testing.T) {
	h := &Handler{ClientID: "id", ClientSecret: "secret", AppID: "42", CallbackURL: "https://api.example.com/callback", FrontendURL: "https://example.com"}
	r := httptest.NewRequest("GET", "/callback?state="+strings.Repeat("a", 43), nil)
	w := httptest.NewRecorder()
	h.Callback(w, r)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
}
func TestGitHubErrorDoesNotExposeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); w.Write([]byte("private-secret")) }))
	defer server.Close()
	h := &Handler{Client: &http.Client{Timeout: time.Second}}
	var out any
	err := h.request(context.Background(), server.URL, "token", nil, &out)
	if err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("unsafe GitHub error")
	}
}
func TestRequestRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat(" ", 2*1024*1024+2))) }))
	defer server.Close()
	h := &Handler{Client: &http.Client{Timeout: time.Second}}
	var out any
	if h.request(context.Background(), server.URL, "", nil, &out) == nil {
		t.Fatal("oversized response accepted")
	}
}
