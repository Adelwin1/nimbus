package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRulesOwnershipAndMonitoring(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app := uuid.New(), uuid.New(), uuid.New()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO users(id,name,email,password_hash)
 VALUES($1,'Owner','owner@rules.test','x'),
 ($2,'Other','other@rules.test','x')`, owner, other)
	exec(`INSERT INTO applications(
 id,user_id,name,application_url,health_url,failure_threshold
 ) VALUES($1,$2,'API','https://example.com',
 'https://example.com/health',1)`, app, owner)

	service := NewService(
		NewRepository(db),
		assertionChecker(200, `{"database":{"connected":false}}`),
	)
	handler := NewHandler(service)
	router := chi.NewRouter()
	router.Get("/apps/{appID}/check-rules", handler.CheckRulesSettings)
	router.Put("/apps/{appID}/check-rules", handler.CheckRulesSettings)

	request := func(user uuid.UUID, method, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(
			method, "/apps/"+app.String()+"/check-rules", strings.NewReader(body),
		)
		req = req.WithContext(context.WithValue(req.Context(), mw.UserIDKey, user))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	payload := `{"expected_status":200,"required_text":"","json_pointer":"/database/connected","json_expected":true}`

	if w := request(other, http.MethodPut, payload); w.Code != 404 {
		t.Fatalf("foreign save: %d %s", w.Code, w.Body.String())
	}
	if w := request(owner, http.MethodPut, payload); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if w := request(other, http.MethodGet, ""); w.Code != 404 {
		t.Fatal("foreign settings disclosed")
	}

	w := request(owner, http.MethodGet, "")
	var saved CheckRules
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &saved) != nil ||
		saved.JSONPointer == nil || string(saved.JSONExpected) != "true" {
		t.Fatal("saved settings did not round-trip")
	}

	check, state, err := service.CheckNow(ctx, app, owner)
	if err != nil {
		t.Fatal(err)
	}
	if check.Healthy || state.Status != "down" || check.ErrorMessage == nil ||
		!strings.Contains(*check.ErrorMessage, "expected value") ||
		!strings.Contains(*check.ErrorMessage, "possible causes") {
		t.Fatalf("saved rule was not applied: %+v %+v", check, state)
	}

	if w := request(owner, http.MethodPut, `{"expected_status":600}`); w.Code != 400 {
		t.Fatal("invalid status accepted")
	}
	if w := request(owner, http.MethodPut,
		`{"json_pointer":null,"json_expected":null}`); w.Code != 200 {
		t.Fatalf("disable: %s", w.Body.String())
	}

	check, _, err = service.CheckNow(ctx, app, owner)
	if err != nil || !check.Healthy {
		t.Fatalf("disabled assertions should restore default check: %+v %v", check, err)
	}
}
