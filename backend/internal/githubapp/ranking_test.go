package githubapp

import (
	"strings"
	"testing"
)

func TestElementFailurePrefersExactPage(t *testing.T) {
	paths := []string{"backend/internal/auth/handler.go", "backend/internal/auth/model.go", "backend/internal/auth/repository.go", "backend/internal/auth/routes.go", "backend/internal/auth/routes_test.go", "frontend/app/login/page.tsx", "frontend/__tests__/pages/login.test.tsx"}
	definition := []byte(`{"steps":[{"action":"navigate","path":"/login"},{"action":"expect_visible","selector":"#missing"}]}`)
	result := []byte(`{"steps":[{"number":1,"action":"navigate","passed":true},{"number":2,"action":"expect_visible","passed":false}],"evidence":{"failed_requests":[]}}`)
	got := rankForFailure(paths, []string{"auth", "login"}, map[string]bool{}, definition, result)
	if len(got) != 5 || got[0].Path != "frontend/app/login/page.tsx" {
		t.Fatalf("page not first: %+v", got)
	}
}
func TestAPIErrorPrefersBackend(t *testing.T) {
	paths := []string{"frontend/app/login/page.tsx", "backend/internal/auth/handler.go"}
	definition := []byte(`{"steps":[{"action":"navigate","path":"/login"},{"action":"expect_visible"}]}`)
	result := []byte(`{"steps":[{"number":2,"action":"expect_visible","passed":false}],"evidence":{"failed_requests":[{"endpoint":"https://example.com/api/login","status":500,"resource_type":"fetch"}]}}`)
	got := rankForFailure(paths, []string{"auth", "login"}, map[string]bool{}, definition, result)
	if !strings.HasPrefix(got[0].Path, "backend/") {
		t.Fatalf("backend not first: %+v", got)
	}
}
func TestAssetFailureDoesNotImplyBackend(t *testing.T) {
	definition := []byte(`{"steps":[{"action":"navigate","path":"/login"},{"action":"expect_visible"}]}`)
	result := []byte(`{"steps":[{"number":2,"action":"expect_visible","passed":false}],"evidence":{"failed_requests":[{"endpoint":"https://example.com/logo.png","status":404,"resource_type":"image"}]}}`)
	if locateFailure(definition, result).APIError {
		t.Fatal("image incorrectly treated as API failure")
	}
}
func TestNavigationAfterFailureIsIgnored(t *testing.T) {
	definition := []byte(`{"steps":[{"action":"navigate","path":"/login?secret=value"},{"action":"expect_visible"},{"action":"navigate","path":"/checkout"}]}`)
	result := []byte(`{"steps":[{"number":2,"action":"expect_visible","passed":false}]}`)
	if got := locateFailure(definition, result).Route; got != "login" {
		t.Fatalf("route=%s", got)
	}
}
