package journeys

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

func TestDefinitionValidation(t *testing.T) {
	value := "Ready"
	good := Definition{
		Name:    "Homepage",
		BaseURL: "https://example.com",
		Steps: []Step{
			{Action: "navigate", Path: "/"},
			{Action: "expect_text", Selector: "h1", Value: &value},
		},
	}
	if !valid(&good) {
		t.Fatal("valid journey rejected")
	}
	for _, tc := range []struct {
		name string
		edit func(*Definition)
	}{
		{"external navigation", func(d *Definition) { d.Steps[0].Path = "https://other.example/" }},
		{"embedded credentials", func(d *Definition) { d.BaseURL = "https://user:password@example.com" }},
		{"unsupported action", func(d *Definition) { d.Steps[1].Action = "execute_script" }},
		{"missing expectation", func(d *Definition) { d.Steps[1].Value = nil }},
		{"missing initial navigation", func(d *Definition) { d.Steps = d.Steps[1:] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := good
			d.Steps = append([]Step(nil), good.Steps...)
			tc.edit(&d)
			if valid(&d) {
				t.Fatal("invalid journey accepted")
			}
		})
	}
}

func TestJourneyOwnershipAndQueue(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app := uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(ctx, `
		INSERT INTO users(id,name,email,password_hash)
		VALUES($1,'Owner','journey-owner@example.com','test'),
		      ($2,'Other','journey-other@example.com','test')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO applications(id,user_id,name,application_url,health_url)
		VALUES($1,$2,'Journey app','https://example.com','https://example.com/health')`,
		app, owner)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{DB: db}
	router := chi.NewRouter()
	router.Get("/apps/{appID}", h.Applications)
	router.Post("/apps/{appID}", h.Applications)
	router.Get("/{journeyID}/runs", h.Runs)
	router.Post("/{journeyID}/runs", h.Runs)
	request := func(user uuid.UUID, method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), mw.UserIDKey, user))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	body := `{"name":"Homepage","base_url":"https://example.com","steps":[{"action":"navigate","path":"/"}]}`
	appPath := "/apps/" + app.String()
	if w := request(other, http.MethodPost, appPath, body); w.Code != 404 {
		t.Fatalf("foreign creation: got %d", w.Code)
	}
	w := request(owner, http.MethodPost, appPath, body)
	if w.Code != 201 {
		t.Fatalf("creation: got %d: %s", w.Code, w.Body.String())
	}
	var saved struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || saved.ID == "" {
		t.Fatal("missing journey ID")
	}
	runPath := "/" + saved.ID + "/runs"
	if w := request(other, http.MethodGet, runPath, ""); w.Code != 404 {
		t.Fatalf("foreign history: got %d", w.Code)
	}
	if w := request(other, http.MethodPost, runPath, ""); w.Code != 404 {
		t.Fatalf("foreign queue: got %d", w.Code)
	}
	if w := request(owner, http.MethodPost, runPath, ""); w.Code != 202 {
		t.Fatalf("queue: got %d: %s", w.Code, w.Body.String())
	}
	if w := request(owner, http.MethodPost, runPath, ""); w.Code != 409 {
		t.Fatalf("duplicate queue: got %d", w.Code)
	}

	_, err = db.Exec(ctx, `UPDATE browser_journeys SET name='Changed' WHERE id=$1`, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var original string
	err = db.QueryRow(ctx, `
		SELECT definition->>'name' FROM browser_journey_runs WHERE journey_id=$1`,
		saved.ID).Scan(&original)
	if err != nil || original != "Homepage" {
		t.Fatalf("run snapshot changed: %q, %v", original, err)
	}
}
