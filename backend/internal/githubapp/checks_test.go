package githubapp

import (
	"archive/zip"
	"bytes"
	"context"
	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepositoryCheckDirectory(t *testing.T) {
	for _, value := range []string{"..", "../backend", "/tmp", "a/../b", "a;ls", ".git", "a b"} {
		if checkDirectory(value) {
			t.Fatal("invalid directory accepted")
		}
	}
	for _, value := range []string{".", "backend", "packages/api"} {
		if !checkDirectory(value) {
			t.Fatal("valid directory rejected")
		}
	}
}
func TestSnapshotRejectsTraversalAndOversize(t *testing.T) {
	archive := func(name, content string) []byte {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		stream, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	if _, err := unpackRepository(archive("root/../outside.js", "bad")); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := unpackRepository(archive("root/file.js", strings.Repeat("a", 1024*1024+1))); err == nil {
		t.Fatal("oversized file accepted")
	}
	files, err := unpackRepository(archive("root/src/index.js", "module.exports=1"))
	if err != nil || len(files) != 1 || files[0].Path != "src/index.js" {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	files, err = unpackRepository(archive("root/.env", "secret"))
	if err != nil || len(files) != 0 {
		t.Fatal("environment file copied")
	}
}
func TestRepositoryChecksRequireRunOwnership(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	testutil.ResetDatabase(t, db)
	ctx := context.Background()
	owner, other, app, journey, run := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err := db.Exec(ctx, `INSERT INTO users(id,name,email,password_hash) VALUES($1,'Owner','checks-owner@example.com','test'),($2,'Other','checks-other@example.com','test')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO applications(id,user_id,name,application_url,health_url) VALUES($1,$2,'Example','https://example.com','https://example.com/health')`, app, owner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO browser_journeys(id,application_id,name,base_url,steps) VALUES($1,$2,'Example','https://example.com','[{"action":"navigate","path":"/"}]')`, journey, app)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO browser_journey_runs(id,journey_id,status,definition) VALUES($1,$2,'failed','{}')`, run, journey)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{DB: db}
	router := chi.NewRouter()
	router.Get("/runs/{runID}/checks", h.ListChecks)
	request := httptest.NewRequest("GET", "/runs/"+run.String()+"/checks", nil)
	request = request.WithContext(context.WithValue(request.Context(), mw.UserIDKey, other))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatalf("foreign run exposed: %d", response.Code)
	}
	request = request.WithContext(context.WithValue(request.Context(), mw.UserIDKey, owner))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"checks":[]`) {
		t.Fatalf("owner results rejected: %d", response.Code)
	}
	check := uuid.New()
	secret := strings.Repeat("s", 43)
	_, err = db.Exec(ctx, `INSERT INTO repository_checks(id,run_id,application_id,user_id,release_context,profile,directory,download_token) VALUES($1,$2,$3,$4,'{}','node-test','.',$5)`, check, run, app, owner, secret)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest("GET", "/runs/"+run.String()+"/checks", nil)
	request = request.WithContext(context.WithValue(request.Context(), mw.UserIDKey, owner))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "download_token") {
		t.Fatal("worker grant exposed in check results")
	}
	t.Setenv("CODE_CHECKS_ENABLED", "true")
	router.Post("/runs/{runID}/checks", h.QueueCheck)
	request = httptest.NewRequest("POST", "/runs/"+run.String()+"/checks", strings.NewReader(`{"profile":"node-test","directory":"."}`))
	request = request.WithContext(context.WithValue(request.Context(), mw.UserIDKey, other))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatal("foreign run queued")
	}
}
