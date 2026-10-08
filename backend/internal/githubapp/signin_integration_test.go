package githubapp

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	appcrypto "github.com/adel/nimbus/backend/internal/crypto"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/adel/nimbus/backend/internal/auth"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestGitHubWorkspaceIdentityAndSingleUseExchange(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	h := &Handler{DB: db}
	first, err := h.resolveIdentity(ctx, 101, "first-login", nil)
	if err != nil {
		t.Fatal(err)
	}
	returning, err := h.resolveIdentity(ctx, 101, "renamed-login", nil)
	if err != nil || returning != first {
		t.Fatalf("returning identity changed: %v", err)
	}
	second, err := h.resolveIdentity(ctx, 202, "second-login", nil)
	if err != nil || second == first {
		t.Fatalf("workspaces not isolated: %v", err)
	}
	if _, err = h.resolveIdentity(ctx, 101, "first-login", &second); err == nil {
		t.Fatal("identity attached to another workspace")
	}
	// Disconnecting GitHub repository access must not reassign the workspace identity.
	if _, err = db.Exec(ctx, `DELETE FROM github_authorizations WHERE user_id=$1`, first); err != nil {
		t.Fatal(err)
	}
	returning, err = h.resolveIdentity(ctx, 101, "first-login", nil)
	if err != nil || returning != first {
		t.Fatal("identity lost after disconnect")
	}
	code := strings.Repeat("a", 43)
	verifier := strings.Repeat("b", 64)
	if _, err = db.Exec(ctx, `INSERT INTO github_login_exchanges(code_hash,challenge,user_id,expires_at) VALUES($1,$2,$3,now()+interval '2 minutes')`, hash(code), hash(verifier), first); err != nil {
		t.Fatal(err)
	}
	if _, err = h.consumeExchange(ctx, code, strings.Repeat("c", 64)); err == nil {
		t.Fatal("another browser redeemed exchange")
	}
	id, err := h.consumeExchange(ctx, code, verifier)
	if err != nil || id != first {
		t.Fatalf("valid exchange rejected: %v", err)
	}
	if _, err = h.consumeExchange(ctx, code, verifier); err == nil {
		t.Fatal("exchange replay accepted")
	}
	code = strings.Repeat("d", 43)
	if _, err = db.Exec(ctx, `INSERT INTO github_login_exchanges(code_hash,challenge,user_id,expires_at) VALUES($1,$2,$3,now()-interval '1 minute')`, hash(code), hash(verifier), first); err != nil {
		t.Fatal(err)
	}
	if _, err = h.consumeExchange(ctx, code, verifier); err == nil {
		t.Fatal("expired exchange accepted")
	}
	service := auth.NewService(auth.NewRepository(db), "access-secret", "refresh-secret", 15*time.Minute, 24*time.Hour)
	session, err := service.CreateExternalSession(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := service.ParseAccessToken(session.AccessToken)
	if err != nil || parsed != first || session.RefreshToken == "" {
		t.Fatalf("invalid external session: %v", err)
	}
}
func TestGitHubSignInPreservesExistingWorkspace(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	h := &Handler{DB: db}
	id := uuid.New()
	if _, err := db.Exec(ctx, `INSERT INTO users(id,name,email,password_hash) VALUES($1,'Original','original@example.com','not-a-real-password')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO github_authorizations(user_id,github_user_id,login,token_encrypted,expires_at) VALUES($1,303,'existing','opaque',now())`, id); err != nil {
		t.Fatal(err)
	}
	got, err := h.resolveIdentity(ctx, 303, "existing", nil)
	if err != nil || got != id {
		t.Fatalf("existing workspace replaced: %v", err)
	}
}

type signInTransport func(*http.Request) (*http.Response, error)

func (f signInTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGitHubSignInCallbackAndExchange(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	enc, err := appcrypto.NewEncryptor(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(auth.NewRepository(db), "access-secret", "refresh-secret", 15*time.Minute, 24*time.Hour)
	h := &Handler{DB: db, Encryptor: enc, Auth: service, ClientID: "client", ClientSecret: "secret", AppID: "1", CallbackURL: "https://api.example.com/api/v1/github/callback", FrontendURL: "https://example.com"}
	h.Client = &http.Client{Transport: signInTransport(func(request *http.Request) (*http.Response, error) {
		body := `{"access_token":"github-token","expires_in":28800}`
		if request.URL.Host == "api.github.com" {
			body = `{"id":404,"login":"new-user"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	verifier := strings.Repeat("b", 64)
	start := httptest.NewRecorder()
	h.LoginStart(start, httptest.NewRequest("GET", "/login?challenge="+hex.EncodeToString(hash(verifier)), nil))
	if start.Code != 303 {
		t.Fatalf("start=%d %s", start.Code, start.Body.String())
	}
	target, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := target.Query().Get("state")
	cookies := start.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("missing protected cookie")
	}
	callbackRequest := httptest.NewRequest("GET", "/callback?state="+state+"&code=provider-code", nil)
	callbackRequest.AddCookie(cookies[0])
	callback := httptest.NewRecorder()
	h.Callback(callback, callbackRequest)
	if callback.Code != 303 {
		t.Fatalf("callback=%d %s", callback.Code, callback.Body.String())
	}
	returned, err := url.Parse(callback.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := returned.Query().Get("code")
	if len(code) != 43 || strings.Contains(returned.String(), "github-token") {
		t.Fatal("invalid or unsafe handoff")
	}
	exchange := httptest.NewRecorder()
	h.LoginExchange(exchange, httptest.NewRequest("POST", "/exchange", strings.NewReader(`{"code":"`+code+`","verifier":"`+verifier+`"}`)))
	if exchange.Code != 200 {
		t.Fatalf("exchange=%d %s", exchange.Code, exchange.Body.String())
	}
	var session auth.AuthResponse
	if err = json.Unmarshal(exchange.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.AccessToken == "" || session.User.ID == uuid.Nil {
		t.Fatal("missing workspace session")
	}
	replay := httptest.NewRecorder()
	h.Callback(replay, callbackRequest)
	if replay.Code != 400 {
		t.Fatal("OAuth state replay accepted")
	}
}
