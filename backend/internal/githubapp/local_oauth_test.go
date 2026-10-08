package githubapp

import "testing"

func TestLocalOAuthRequiresExplicitDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("GITHUB_ALLOW_LOCAL_DEVELOPMENT", "true")
	h := &Handler{ClientID: "id", ClientSecret: "secret", AppID: "1", CallbackURL: "http://localhost:8080/api/v1/github/callback", FrontendURL: "http://localhost:3000"}
	if !h.configured() {
		t.Fatal("local development rejected")
	}
	cookie := h.oauthCookie("nonce", 600)
	if cookie.Secure || !cookie.HttpOnly || cookie.Name != "nimbus-github-local" {
		t.Fatal("wrong development cookie")
	}
	t.Setenv("APP_ENV", "production")
	if h.configured() {
		t.Fatal("production allowed insecure callback")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("GITHUB_ALLOW_LOCAL_DEVELOPMENT", "false")
	if h.configured() {
		t.Fatal("local OAuth enabled without opt-in")
	}
	t.Setenv("GITHUB_ALLOW_LOCAL_DEVELOPMENT", "true")
	h.CallbackURL = "http://example.com/api/v1/github/callback"
	if h.configured() {
		t.Fatal("non-loopback HTTP callback accepted")
	}
	h.CallbackURL = "http://localhost:8080/api/v1/github/callback"
	h.FrontendURL = "http://example.com"
	if h.configured() {
		t.Fatal("non-loopback frontend accepted")
	}
	h.CallbackURL = "https://api.example.com/api/v1/github/callback"
	h.FrontendURL = "https://example.com"
	if !h.configured() || !h.oauthCookie("nonce", 600).Secure || h.oauthCookieName() != "__Host-nimbus-github" {
		t.Fatal("HTTPS protections lost")
	}
}
