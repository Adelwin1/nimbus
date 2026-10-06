package githubapp

import (
	"encoding/json"
	"testing"
)

func TestSourceRanking(t *testing.T) {
	got := rankSources([]string{".env.ts", "node_modules/auth.ts", "frontend/app/login/page.tsx", "backend/internal/auth/service.go", "other.go"}, []string{"login", "auth"}, map[string]bool{"backend/internal/auth/service.go": true, "other.go": true})
	if len(got) != 2 || got[0].Path != "backend/internal/auth/service.go" {
		t.Fatalf("unexpected candidates: %+v", got)
	}
}
func TestSearchTermsExcludeSecrets(t *testing.T) {
	definition := []byte(`{"steps":[{"action":"navigate","path":"/login?token=secret"},{"action":"fill","value":"password-secret"}]}`)
	report := []byte(`{"evidence":{"failed_requests":[{"endpoint":"https://example.com/api/login?secret=value"}]}}`)
	terms := searchTerms(definition, report)
	encoded, _ := json.Marshal(terms)
	if string(encoded) != `["api","auth","login"]` {
		t.Fatalf("unsafe or unexpected terms: %s", encoded)
	}
}
