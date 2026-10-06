package journeys

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

type RunInput struct {
	CommitSHA  string `json:"commit_sha"`
	PreviewURL string `json:"preview_url"`
}

// Empty input preserves existing ad-hoc runs. Release runs require both fields.
func validRelease(input *RunInput) bool {
	input.CommitSHA = strings.TrimSpace(input.CommitSHA)
	input.PreviewURL = strings.TrimSpace(input.PreviewURL)
	if input.CommitSHA == "" && input.PreviewURL == "" {
		return true
	}
	if !fullCommit.MatchString(input.CommitSHA) || len(input.PreviewURL) > 2048 {
		return false
	}
	u, e := url.Parse(input.PreviewURL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return false
	}
	return true
}
