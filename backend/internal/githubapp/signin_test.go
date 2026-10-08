package githubapp

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginChallengeValidation(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("a", 63), strings.Repeat("z", 64)} {
		if _, err := validChallenge(value); err == nil {
			t.Fatal("invalid challenge accepted")
		}
	}
	if b, err := validChallenge(strings.Repeat("a", 64)); err != nil || len(b) != 32 {
		t.Fatal("valid challenge rejected")
	}
}
func TestLoginStartRejectsUnconfiguredService(t *testing.T) {
	h := &Handler{}
	w := httptest.NewRecorder()
	h.LoginStart(w, httptest.NewRequest("GET", "/login", nil))
	if w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
}
