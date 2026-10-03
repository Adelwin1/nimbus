package insights

import (
	"context"
	"errors"
	middleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	called bool
	user   uuid.UUID
	err    error
}

func (s *fakeStore) Get(_ context.Context, id uuid.UUID, now time.Time) (Overview, error) {
	s.called = true
	s.user = id
	return Overview{GeneratedAt: now, History: []Bucket{}, Releases: []Release{}, Incidents: []Incident{}}, s.err
}
func TestOverviewRequiresIdentity(t *testing.T) {
	s := &fakeStore{}
	w := httptest.NewRecorder()
	Handler{s}.Overview(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusUnauthorized || s.called {
		t.Fatal("unauthenticated request reached the store")
	}
}
func TestOverviewUsesAuthenticatedTenant(t *testing.T) {
	s := &fakeStore{}
	id := uuid.New()
	r := httptest.NewRequest("GET", "/?user_id="+uuid.NewString(), nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, id))
	w := httptest.NewRecorder()
	Handler{s}.Overview(w, r)
	if w.Code != 200 || s.user != id {
		t.Fatal("wrong tenant")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("analytics must not be cached publicly")
	}
	if !strings.Contains(w.Body.String(), `"success_percent":null`) {
		t.Fatal("missing data must not become 0% or 100%")
	}
}
func TestOverviewHidesStoreErrors(t *testing.T) {
	s := &fakeStore{err: errors.New("secret database password")}
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uuid.New()))
	w := httptest.NewRecorder()
	Handler{s}.Overview(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("database error leaked")
	}
}
