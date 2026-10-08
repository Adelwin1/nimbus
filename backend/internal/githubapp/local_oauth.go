package githubapp

import (
	"net/http"
	"net/url"
	"os"
)

func loopbackURL(raw string, callback bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return false
	}
	if callback {
		return u.Path == "/api/v1/github/callback"
	}
	return u.Path == "" || u.Path == "/"
}
func (h *Handler) localOAuth() bool {
	return os.Getenv("APP_ENV") == "development" && os.Getenv("GITHUB_ALLOW_LOCAL_DEVELOPMENT") == "true" && loopbackURL(h.CallbackURL, true) && loopbackURL(h.FrontendURL, false)
}
func (h *Handler) configured() bool {
	return h.productionConfigured() || (h.ClientID != "" && h.ClientSecret != "" && h.AppID != "" && h.localOAuth())
}
func (h *Handler) oauthCookieName() string {
	if h.localOAuth() {
		return "nimbus-github-local"
	}
	return "__Host-nimbus-github"
}
func (h *Handler) oauthCookie(value string, age int) *http.Cookie {
	return &http.Cookie{Name: h.oauthCookieName(), Value: value, Path: "/", Secure: !h.localOAuth(), HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age}
}
