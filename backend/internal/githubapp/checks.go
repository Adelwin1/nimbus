package githubapp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/adel/nimbus/backend/internal/workerdispatch"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func checkDirectory(value string) bool {
	if value == "." {
		return true
	}
	if value == "" || len(value) > 180 || path.Clean(value) != value || strings.HasPrefix(value, "/") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." || segment == "" || strings.HasPrefix(segment, ".") {
			return false
		}
		for _, char := range segment {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
				return false
			}
		}
	}
	return true
}
func (h *Handler) CheckRunOwner(r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, []byte, error) {
	user, ok := mw.GetUserID(r.Context())
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, nil, errors.New("authentication required")
	}
	run, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		return user, uuid.Nil, uuid.Nil, nil, err
	}
	var app uuid.UUID
	var release []byte
	err = h.DB.QueryRow(r.Context(), `SELECT j.application_id,r.release_context FROM browser_journey_runs r JOIN browser_journeys j ON j.id=r.journey_id JOIN applications a ON a.id=j.application_id WHERE r.id=$1 AND a.user_id=$2`, run, user).Scan(&app, &release)
	return user, run, app, release, err
}
func (h *Handler) QueueCheck(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("CODE_CHECKS_ENABLED") != "true" {
		fail(w, 503, "Repository checks are not enabled on this server.")
		return
	}
	user, run, app, release, err := h.CheckRunOwner(r)
	if err != nil {
		fail(w, 404, "Journey run not found.")
		return
	}
	var input struct {
		Profile   string `json:"profile"`
		Directory string `json:"directory"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || !checkDirectory(input.Directory) || (input.Profile != "node-test" && input.Profile != "go-test") {
		fail(w, 400, "Choose a supported profile and repository directory.")
		return
	}
	var captured struct {
		Commit         string `json:"commit_sha"`
		RepositoryID   int64  `json:"repository_id"`
		InstallationID int64  `json:"installation_id"`
	}
	_ = json.Unmarshal(release, &captured)
	verified, err := h.VerifyCommit(r, app, captured.Commit)
	if err != nil || verified["repository_id"] != captured.RepositoryID || verified["installation_id"] != captured.InstallationID {
		fail(w, 409, "Record a commit for this journey and verify the linked repository access first.")
		return
	}
	grant, err := nonce()
	if err != nil {
		fail(w, 500, "Check unavailable.")
		return
	}
	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		fail(w, 500, "Check unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "nimbus-check-"+user.String())
	var count int
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT count(*) FROM repository_checks WHERE user_id=$1 AND status IN ('queued','running')`, user).Scan(&count)
	}
	if err != nil {
		fail(w, 500, "Check unavailable.")
		return
	}
	if count >= 2 {
		fail(w, 429, "Wait for your active repository checks to finish.")
		return
	}
	id := uuid.New()
	if err = workerdispatch.Reserve(r.Context(), tx, user, id, "repository"); err != nil {
		fail(w, 429, err.Error())
		return
	}
	err = tx.QueryRow(r.Context(), `INSERT INTO repository_checks(id,run_id,application_id,user_id,release_context,profile,directory,download_token) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, id, run, app, user, release, input.Profile, input.Directory, grant).Scan(&id)
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		fail(w, 500, "Check unavailable.")
		return
	}
	if err = workerdispatch.Dispatch(r.Context(), "repository", id); err != nil {
		_, _ = h.DB.Exec(context.Background(), `UPDATE repository_checks SET status='error',error_message=$2,finished_at=now(),download_token=NULL WHERE id=$1 AND status='queued'`, id, err.Error())
		fail(w, 503, err.Error())
		return
	}
	reply(w, 202, map[string]any{"id": id, "status": "queued"})
}
func (h *Handler) ListChecks(w http.ResponseWriter, r *http.Request) {
	user, run, _, _, err := h.CheckRunOwner(r)
	if err != nil {
		fail(w, 404, "Journey run not found.")
		return
	}
	_, _ = h.DB.Exec(r.Context(), `UPDATE repository_checks SET status='error',error_message='Hosted job did not start or finish. Please run a new check.',finished_at=now(),download_token=NULL,lease_token=NULL,lease_expires_at=NULL WHERE run_id=$1 AND user_id=$2 AND ((status='queued' AND queued_at<now()-interval '20 minutes') OR (status='running' AND lease_expires_at<now()))`, run, user)
	rows, err := h.DB.Query(r.Context(), `SELECT id,profile,directory,status,result,error_message,queued_at FROM repository_checks WHERE run_id=$1 AND user_id=$2 ORDER BY queued_at DESC LIMIT 10`, run, user)
	if err != nil {
		fail(w, 500, "Checks unavailable.")
		return
	}
	defer rows.Close()
	checks := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var profile, directory, status string
		var result []byte
		var message *string
		var queued time.Time
		if rows.Scan(&id, &profile, &directory, &status, &result, &message, &queued) != nil {
			fail(w, 500, "Checks unavailable.")
			return
		}
		var parsed any
		if len(result) > 0 {
			_ = json.Unmarshal(result, &parsed)
		}
		checks = append(checks, map[string]any{"id": id, "profile": profile, "directory": directory, "status": status, "result": parsed, "error_message": message, "queued_at": queued})
	}
	if rows.Err() != nil {
		fail(w, 500, "Checks unavailable.")
		return
	}
	reply(w, 200, map[string]any{"checks": checks})
}

type repositoryFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func unpackRepository(data []byte) ([]repositoryFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) > 5000 {
		return nil, errors.New("repository has too many files")
	}
	files := []repositoryFile{}
	total := 0
	seen := map[string]bool{}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if !file.Mode().IsRegular() {
			return nil, errors.New("links and special files are not supported")
		}
		parts := strings.SplitN(file.Name, "/", 2)
		if len(parts) != 2 {
			return nil, errors.New("invalid archive layout")
		}
		relative := parts[1]
		if relative == "" || path.Clean(relative) != relative || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\\") {
			return nil, errors.New("invalid repository path")
		}
		for _, segment := range strings.Split(relative, "/") {
			if segment == ".." || segment == "." || segment == "" {
				return nil, errors.New("invalid repository path")
			}
		}
		if seen[relative] {
			return nil, errors.New("duplicate repository path")
		}
		seen[relative] = true
		if file.UncompressedSize64 > 1024*1024 {
			return nil, errors.New("a repository file exceeds 1 MB")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(stream, 1024*1024+1))
		stream.Close()
		if err != nil || len(content) > 1024*1024 {
			return nil, errors.New("repository file invalid")
		}
		total += len(content)
		if total > 10*1024*1024 {
			return nil, errors.New("repository exceeds 10 MB execution limit")
		}
		// Environment files are not copied into test containers.
		if strings.HasPrefix(path.Base(relative), ".env") {
			continue
		}
		files = append(files, repositoryFile{Path: relative, Content: base64.StdEncoding.EncodeToString(content)})
	}
	return files, nil
}
func (h *Handler) downloadSnapshot(ctx context.Context, repository, commit, token string) ([]repositoryFile, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, errors.New("invalid repository")
	}
	endpoint := "https://api.github.com/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/zipball/" + commit
	request, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", "Nimbus")
	response, err := h.Client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == 302 {
		location := response.Header.Get("Location")
		response.Body.Close()
		target, err := url.Parse(location)
		if err != nil || target.Scheme != "https" || target.Host != "codeload.github.com" || target.User != nil {
			return nil, errors.New("archive redirect rejected")
		}
		// Do not forward the GitHub authorization header to the archive host.
		request, err = http.NewRequestWithContext(ctx, "GET", location, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "Nimbus")
		response, err = h.Client.Do(request)
		if err != nil {
			return nil, err
		}
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("archive unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 20*1024*1024+1))
	if err != nil || len(data) > 20*1024*1024 {
		return nil, errors.New("archive too large")
	}
	return unpackRepository(data)
}
func (h *Handler) WorkerSource(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("CODE_CHECKS_ENABLED") != "true" {
		fail(w, 503, "Repository checks disabled.")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "checkID"))
	if err != nil {
		fail(w, 404, "Check not found.")
		return
	}
	grant := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(grant) != 43 {
		fail(w, 403, "Invalid worker grant.")
		return
	}
	var expected string
	var user, app uuid.UUID
	var release []byte
	err = h.DB.QueryRow(r.Context(), `SELECT download_token,user_id,application_id,release_context FROM repository_checks WHERE id=$1 AND status='running' AND lease_expires_at>now() AND download_token IS NOT NULL`, id).Scan(&expected, &user, &app, &release)
	if err != nil || subtle.ConstantTimeCompare([]byte(grant), []byte(expected)) != 1 {
		fail(w, 403, "Worker grant expired.")
		return
	}
	var captured struct {
		Commit         string `json:"commit_sha"`
		RepositoryID   int64  `json:"repository_id"`
		InstallationID int64  `json:"installation_id"`
	}
	_ = json.Unmarshal(release, &captured)
	owned := r.WithContext(context.WithValue(r.Context(), mw.UserIDKey, user))
	verified, err := h.VerifyCommit(owned, app, captured.Commit)
	if err != nil || verified["repository_id"] != captured.RepositoryID || verified["installation_id"] != captured.InstallationID {
		fail(w, 403, "Repository access changed. Reconnect and queue a new check.")
		return
	}
	_, token, err := h.token(owned)
	if err != nil {
		fail(w, 403, "GitHub authorization expired.")
		return
	}
	tag, err := h.DB.Exec(r.Context(), `UPDATE repository_checks SET download_token=NULL WHERE id=$1 AND download_token=$2 AND status='running' AND lease_expires_at>now()`, id, grant)
	if err != nil || tag.RowsAffected() != 1 {
		fail(w, 403, "Worker grant already used.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	files, err := h.downloadSnapshot(ctx, verified["repository"].(string), captured.Commit, token)
	if err != nil {
		fail(w, 502, "Repository snapshot unavailable or exceeds execution limits.")
		return
	}
	reply(w, 200, map[string]any{"files": files, "commit_sha": captured.Commit, "repository": verified["repository"]})
}
