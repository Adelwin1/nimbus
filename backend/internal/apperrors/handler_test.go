package apperrors

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestStructuredErrorValidation(t *testing.T) {
	good := Event{
		Type: "DatabaseError", Code: "DB_UNAVAILABLE", Release: "v1.2.0",
		Frames: []Frame{{File: "src/database.go", Function: "connect", Line: 42}},
	}
	if !valid(&good) {
		t.Fatal("valid event rejected")
	}
	bad := good
	bad.Frames = []Frame{{File: "/Users/private/database.go", Function: "connect", Line: 42}}
	if valid(&bad) {
		t.Fatal("absolute local path accepted")
	}
	bad = good
	bad.Type = "error containing arbitrary message"
	if valid(&bad) {
		t.Fatal("arbitrary error message accepted")
	}
}

func TestReportingTokensAndGrouping(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app := uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(ctx, `
		INSERT INTO users(id,name,email,password_hash)
		VALUES($1,'Owner','errors-owner@example.com','test'),
		      ($2,'Other','errors-other@example.com','test')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO applications(id,user_id,name,application_url,health_url)
		VALUES($1,$2,'Errors app','https://example.com','https://example.com/health')`,
		app, owner)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db}
	router := chi.NewRouter()
	router.Post("/apps/{appID}/token", h.Token)
	router.Get("/apps/{appID}", h.List)
	router.Post("/reports/{appID}", h.Report)

	request := func(user uuid.UUID, method, target, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if user != uuid.Nil {
			r = r.WithContext(context.WithValue(r.Context(), mw.UserIDKey, user))
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	appPath := "/apps/" + app.String()
	reportPath := "/reports/" + app.String()
	if w := request(other, "POST", appPath+"/token", "", ""); w.Code != 404 {
		t.Fatalf("foreign token creation: %d", w.Code)
	}
	newToken := func() string {
		t.Helper()
		w := request(owner, "POST", appPath+"/token", "", "")
		if w.Code != 201 {
			t.Fatalf("token creation: %d", w.Code)
		}
		var payload struct {
			Token string `json:"token"`
		}
		if json.Unmarshal(w.Body.Bytes(), &payload) != nil || payload.Token == "" {
			t.Fatal("token missing")
		}
		return payload.Token
	}
	oldToken := newToken()
	body := `{"error_type":"DatabaseError","error_code":"DB_UNAVAILABLE","release_version":"v1","frames":[{"file":"src/database.go","function":"connect","line":42}]}`
	if w := request(uuid.Nil, "POST", reportPath, "", body); w.Code != 401 {
		t.Fatalf("unauthenticated reporting: %d", w.Code)
	}
	if w := request(uuid.Nil, "POST", reportPath, oldToken, body); w.Code != 202 {
		t.Fatalf("first report: %d", w.Code)
	}
	bodyV2 := strings.Replace(body, `"v1"`, `"v2"`, 1)
	if w := request(uuid.Nil, "POST", reportPath, oldToken, bodyV2); w.Code != 202 {
		t.Fatalf("second report: %d", w.Code)
	}
	if w := request(other, "GET", appPath, "", ""); w.Code != 404 {
		t.Fatalf("foreign error access: %d", w.Code)
	}

	var groups, occurrences int
	if err := db.QueryRow(ctx, `
		SELECT COUNT(*),COALESCE(SUM(occurrences),0)
		FROM application_error_groups WHERE application_id=$1`, app).
		Scan(&groups, &occurrences); err != nil {
		t.Fatal(err)
	}
	if groups != 1 || occurrences != 2 {
		t.Fatalf("grouping incorrect: %d groups, %d occurrences", groups, occurrences)
	}
	token := newToken()
	if w := request(uuid.Nil, "POST", reportPath, oldToken, body); w.Code != 401 {
		t.Fatalf("rotated token still accepted: %d", w.Code)
	}
	if w := request(uuid.Nil, "POST", reportPath, token, body); w.Code != 202 {
		t.Fatalf("new token rejected: %d", w.Code)
	}
	invalid := `{"error_type":"DatabaseError","error_code":"DB_UNAVAILABLE","password":"secret"}`
	if w := request(uuid.Nil, "POST", reportPath, token, invalid); w.Code != 400 {
		t.Fatalf("unexpected secret field accepted: %d", w.Code)
	}
}
