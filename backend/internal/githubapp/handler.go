package githubapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	appcrypto "github.com/adel/nimbus/backend/internal/crypto"
	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	DB                                                            *pgxpool.Pool
	Encryptor                                                     *appcrypto.Encryptor
	Client                                                        *http.Client
	ClientID, ClientSecret, AppID, Slug, CallbackURL, FrontendURL string
}

func New(db *pgxpool.Pool, enc *appcrypto.Encryptor) *Handler {
	return &Handler{DB: db, Encryptor: enc, Client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		ClientID: os.Getenv("GITHUB_CLIENT_ID"), ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"), AppID: os.Getenv("GITHUB_APP_ID"), Slug: os.Getenv("GITHUB_APP_SLUG"), CallbackURL: os.Getenv("GITHUB_CALLBACK_URL"), FrontendURL: strings.TrimRight(os.Getenv("FRONTEND_ORIGIN"), "/")}
}
func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"message": message}})
}
func nonce() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func hash(s string) []byte { v := sha256.Sum256([]byte(s)); return v[:] }
func (h *Handler) configured() bool {
	u, e := url.Parse(h.CallbackURL)
	f, fe := url.Parse(h.FrontendURL)
	return h.ClientID != "" && h.ClientSecret != "" && h.AppID != "" && e == nil && fe == nil && u.Scheme == "https" && u.Host != "" && f.Scheme == "https" && f.Host != "" && u.User == nil && f.User == nil && f.RawQuery == "" && f.Fragment == ""
}
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	if !h.configured() {
		fail(w, 503, "GitHub connection is not configured.")
		return
	}
	ticket, e := nonce()
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	// Limit outstanding tickets per account and remove expired states.
	tx, e := h.DB.Begin(r.Context())
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `DELETE FROM github_oauth_states WHERE user_id=$1 OR expires_at<now()`, id)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO github_oauth_states(state_hash,user_id,phase,expires_at) VALUES($1,$2,'ticket',now()+interval '5 minutes')`, hash(ticket), id)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	u, _ := url.Parse(h.CallbackURL)
	u.Path = "/api/v1/github/authorize"
	u.RawQuery = url.Values{"ticket": {ticket}}.Encode()
	reply(w, 200, map[string]string{"url": u.String()})
}
func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		fail(w, 503, "GitHub connection is not configured.")
		return
	}
	ticket := r.URL.Query().Get("ticket")
	if len(ticket) != 43 {
		fail(w, 400, "Invalid connection link.")
		return
	}
	state, e := nonce()
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	var id uuid.UUID
	e = h.DB.QueryRow(r.Context(), `UPDATE github_oauth_states SET state_hash=$2,phase='oauth',expires_at=now()+interval '10 minutes' WHERE state_hash=$1 AND phase='ticket' AND expires_at>now() RETURNING user_id`, hash(ticket), hash(state)).Scan(&id)
	if e != nil {
		fail(w, 400, "Connection link expired. Start again in Nimbus.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "__Host-nimbus-github", Value: state, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	q := url.Values{"client_id": {h.ClientID}, "redirect_uri": {h.CallbackURL}, "state": {state}}
	http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+q.Encode(), http.StatusSeeOther)
}
func (h *Handler) request(ctx context.Context, endpoint, token string, body any, out any) error {
	var reader io.Reader
	method := "GET"
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(b)
		method = "POST"
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if e != nil {
		return e
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Nimbus")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := h.Client.Do(req)
	if e != nil {
		return errors.New("GitHub unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("GitHub status %d", res.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if e != nil || len(b) > 2*1024*1024 {
		return errors.New("GitHub response invalid")
	}
	return json.Unmarshal(b, out)
}
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		fail(w, 503, "GitHub connection is not configured.")
		return
	}
	state := r.URL.Query().Get("state")
	cookie, e := r.Cookie("__Host-nimbus-github")
	if e != nil || len(state) != 43 || subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		fail(w, 400, "Invalid GitHub authorization. Start from Nimbus.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "__Host-nimbus-github", Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	var id uuid.UUID
	e = h.DB.QueryRow(r.Context(), `DELETE FROM github_oauth_states WHERE state_hash=$1 AND phase='oauth' AND expires_at>now() RETURNING user_id`, hash(state)).Scan(&id)
	if e != nil {
		fail(w, 400, "Authorization expired or already used.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 1024 {
		fail(w, 400, "GitHub authorization was cancelled.")
		return
	}
	var result struct {
		Token   string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	e = h.request(r.Context(), "https://github.com/login/oauth/access_token", "", map[string]string{"client_id": h.ClientID, "client_secret": h.ClientSecret, "code": code, "redirect_uri": h.CallbackURL}, &result)
	if e != nil || result.Token == "" || result.Expires < 1 || result.Expires > 86400 {
		fail(w, 502, "GitHub authorization failed. Enable expiring user tokens and try again.")
		return
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	e = h.request(r.Context(), "https://api.github.com/user", result.Token, nil, &user)
	if e != nil || user.ID < 1 || user.Login == "" {
		fail(w, 502, "GitHub identity could not be verified.")
		return
	}
	encrypted, e := h.Encryptor.Encrypt(result.Token)
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	_, e = h.DB.Exec(r.Context(), `INSERT INTO github_authorizations(user_id,github_user_id,login,token_encrypted,expires_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id) DO UPDATE SET github_user_id=EXCLUDED.github_user_id,login=EXCLUDED.login,token_encrypted=EXCLUDED.token_encrypted,expires_at=EXCLUDED.expires_at,connected_at=now()`, id, user.ID, user.Login, encrypted, time.Now().Add(time.Duration(result.Expires)*time.Second))
	if e != nil {
		fail(w, 500, "Connection could not be saved.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.FrontendURL+"/integrations?github=connected", http.StatusSeeOther)
}
func (h *Handler) token(r *http.Request) (uuid.UUID, string, error) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		return id, "", errors.New("unauthorized")
	}
	var encrypted string
	e := h.DB.QueryRow(r.Context(), `SELECT token_encrypted FROM github_authorizations WHERE user_id=$1 AND expires_at>now()`, id).Scan(&encrypted)
	if e != nil {
		return id, "", e
	}
	t, e := h.Encryptor.Decrypt(encrypted)
	return id, t, e
}
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	var login string
	var expiry time.Time
	e := h.DB.QueryRow(r.Context(), `SELECT login,expires_at FROM github_authorizations WHERE user_id=$1`, id).Scan(&login, &expiry)
	installURL := "https://github.com/apps/" + url.PathEscape(h.Slug) + "/installations/new"
	reply(w, 200, map[string]any{"configured": h.configured(), "connected": e == nil && expiry.After(time.Now()), "login": login, "expires_at": expiry, "install_url": installURL})
}
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	id, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	tx, e := h.DB.Begin(r.Context())
	if e != nil {
		fail(w, 500, "Disconnect failed.")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `DELETE FROM application_github_repositories WHERE application_id IN(SELECT id FROM applications WHERE user_id=$1)`, id)
	if e == nil {
		_, e = tx.Exec(r.Context(), `DELETE FROM github_authorizations WHERE user_id=$1`, id)
	}
	if e == nil {
		_, e = tx.Exec(r.Context(), `DELETE FROM github_oauth_states WHERE user_id=$1`, id)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		fail(w, 500, "Disconnect failed.")
		return
	}
	reply(w, 200, map[string]bool{"disconnected": true})
}

type installation struct {
	ID        int64   `json:"id"`
	AppID     int64   `json:"app_id"`
	Suspended *string `json:"suspended_at"`
	Account   struct {
		Login string `json:"login"`
	} `json:"account"`
}
type repository struct {
	ID             int64  `json:"id"`
	Name           string `json:"full_name"`
	Branch         string `json:"default_branch"`
	InstallationID int64  `json:"installation_id"`
}

func (h *Handler) repositories(ctx context.Context, token string) ([]repository, error) {
	repos := []repository{}
	appID, e := strconv.ParseInt(h.AppID, 10, 64)
	if e != nil {
		return nil, e
	}
	// Bound API fanout; ask the user to narrow installations instead of silently truncating.
	for page := 1; page <= 10; page++ {
		var data struct {
			Installations []installation `json:"installations"`
		}
		e = h.request(ctx, fmt.Sprintf("https://api.github.com/user/installations?per_page=100&page=%d", page), token, nil, &data)
		if e != nil {
			return nil, e
		}
		for _, i := range data.Installations {
			if i.AppID != appID || i.Suspended != nil {
				continue
			}
			for rp := 1; rp <= 10; rp++ {
				var list struct {
					Repositories []repository `json:"repositories"`
				}
				e = h.request(ctx, fmt.Sprintf("https://api.github.com/user/installations/%d/repositories?per_page=100&page=%d", i.ID, rp), token, nil, &list)
				if e != nil {
					return nil, e
				}
				for _, repo := range list.Repositories {
					repo.InstallationID = i.ID
					repos = append(repos, repo)
				}
				if len(list.Repositories) < 100 {
					break
				}
				if rp == 10 {
					return nil, errors.New("too many repositories")
				}
			}
		}
		if len(data.Installations) < 100 {
			return repos, nil
		}
	}
	return nil, errors.New("too many installations")
}
func (h *Handler) Repositories(w http.ResponseWriter, r *http.Request) {
	_, token, e := h.token(r)
	if e != nil {
		fail(w, 409, "Connect or reauthorize GitHub first.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	repos, e := h.repositories(ctx, token)
	if e != nil {
		fail(w, 502, "Repositories unavailable. Check installation access or reconnect.")
		return
	}
	reply(w, 200, map[string]any{"repositories": repos})
}
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	id, token, e := h.token(r)
	if e != nil {
		fail(w, 409, "Connect or reauthorize GitHub first.")
		return
	}
	app, e := uuid.Parse(chi.URLParam(r, "appID"))
	if e != nil {
		fail(w, 400, "Invalid application.")
		return
	}
	var owns bool
	e = h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM applications WHERE id=$1 AND user_id=$2)`, app, id).Scan(&owns)
	if e != nil {
		fail(w, 500, "Connection unavailable.")
		return
	}
	if !owns {
		fail(w, 404, "Application not found.")
		return
	}
	var input struct {
		RepositoryID   int64 `json:"repository_id"`
		InstallationID int64 `json:"installation_id"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&input) != nil || d.Decode(&extra) != io.EOF || input.RepositoryID < 1 || input.InstallationID < 1 {
		fail(w, 400, "Invalid repository selection.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	repos, e := h.repositories(ctx, token)
	if e != nil {
		fail(w, 502, "GitHub access could not be verified.")
		return
	}
	for _, repo := range repos {
		if repo.ID == input.RepositoryID && repo.InstallationID == input.InstallationID {
			tag, err := h.DB.Exec(r.Context(), `INSERT INTO application_github_repositories(application_id,installation_id,repository_id,full_name,default_branch) SELECT id,$3,$4,$5,$6 FROM applications WHERE id=$1 AND user_id=$2 ON CONFLICT(application_id) DO UPDATE SET installation_id=EXCLUDED.installation_id,repository_id=EXCLUDED.repository_id,full_name=EXCLUDED.full_name,default_branch=EXCLUDED.default_branch,linked_at=now()`, app, id, repo.InstallationID, repo.ID, repo.Name, repo.Branch)
			if err != nil {
				fail(w, 500, "Repository could not be linked.")
				return
			}
			if tag.RowsAffected() == 0 {
				fail(w, 404, "Application not found.")
				return
			}
			reply(w, 200, map[string]any{"linked": true, "repository": repo})
			return
		}
	}
	fail(w, 403, "The selected repository is not accessible to your GitHub account and this app.")
}
