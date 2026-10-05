package repairs

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

func TestRepairValidation(t *testing.T) {
	patch := "diff --git a/backend/internal/middleware/cors.go b/backend/internal/middleware/cors.go\n" +
		"--- a/backend/internal/middleware/cors.go\n" +
		"+++ b/backend/internal/middleware/cors.go\n" +
		"@@ -1 +1 @@\n-old\n+new\n"
	input := Input{
		Rule:       "go-cors-route-method-mismatch",
		BaseCommit: strings.Repeat("a", 40),
		Patch:      patch,
	}
	if !valid(input) {
		t.Fatal("unverified proposal should be importable")
	}
	input.Validation.ChecksPassed = true
	if valid(input) {
		t.Fatal("passing claim accepted without checks")
	}
	input.Validation.ChecksPassed = false
	input.Patch += "diff --git a/other.go b/other.go\n"
	if valid(input) {
		t.Fatal("multi-file patch accepted")
	}
}

func TestRepairOwnershipAndDecision(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app := uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(ctx, `
		INSERT INTO users(id,name,email,password_hash)
		VALUES($1,'Owner','repair-owner@example.com','test'),
		      ($2,'Other','repair-other@example.com','test')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO applications(id,user_id,name,application_url,health_url)
		VALUES($1,$2,'Repair app','https://example.com','https://example.com/health')`,
		app, owner)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{DB: db}
	router := chi.NewRouter()
	router.Get("/apps/{appID}", h.Reviews)
	router.Post("/apps/{appID}", h.Reviews)
	router.Post("/{reviewID}/decision", h.Decide)
	request := func(user uuid.UUID, method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), mw.UserIDKey, user))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}

	input := Input{
		Rule:       "go-cors-route-method-mismatch",
		BaseCommit: strings.Repeat("a", 40),
		Patch: "diff --git a/backend/internal/middleware/cors.go b/backend/internal/middleware/cors.go\n" +
			"--- a/backend/internal/middleware/cors.go\n" +
			"+++ b/backend/internal/middleware/cors.go\n" +
			"@@ -1 +1 @@\n-old\n+new\n",
	}
	body, _ := json.Marshal(input)
	appPath := "/apps/" + app.String()
	if w := request(other, "POST", appPath, string(body)); w.Code != 404 {
		t.Fatalf("foreign import: %d", w.Code)
	}
	w := request(owner, "POST", appPath, string(body))
	if w.Code != 201 {
		t.Fatalf("import: %d", w.Code)
	}
	var saved struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(w.Body.Bytes(), &saved) != nil || saved.ID == "" {
		t.Fatal("missing review ID")
	}
	if w := request(other, "GET", appPath, ""); w.Code != 404 {
		t.Fatalf("foreign list: %d", w.Code)
	}
	decisionPath := "/" + saved.ID + "/decision"
	if w := request(other, "POST", decisionPath, `{"decision":"approved"}`); w.Code != 404 {
		t.Fatalf("foreign decision: %d", w.Code)
	}
	if w := request(owner, "POST", decisionPath, `{"decision":"rejected"}`); w.Code != 200 {
		t.Fatalf("rejection: %d", w.Code)
	}
	if w := request(owner, "POST", decisionPath, `{"decision":"approved"}`); w.Code != 409 {
		t.Fatalf("decision overwritten: %d", w.Code)
	}
	var status, hash string
	var reviewed bool
	err = db.QueryRow(ctx, `
		SELECT review_status,patch_sha256,reviewed_at IS NOT NULL
		FROM application_repair_reviews WHERE id=$1`, saved.ID).
		Scan(&status, &hash, &reviewed)
	if err != nil || status != "rejected" || len(hash) != 64 || !reviewed {
		t.Fatalf("incorrect stored review: %v", err)
	}
}
