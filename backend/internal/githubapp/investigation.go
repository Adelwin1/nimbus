package githubapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	mw "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type sourceFile struct {
	Path    string   `json:"path"`
	URL     string   `json:"url"`
	Score   int      `json:"score"`
	Changed bool     `json:"changed_since_baseline"`
	Lines   []int    `json:"matching_lines"`
	Reasons []string `json:"reasons"`
}
type investigation struct {
	Commit      string       `json:"commit_sha"`
	Repository  string       `json:"repository"`
	Baseline    string       `json:"baseline_commit,omitempty"`
	Candidates  []sourceFile `json:"candidates"`
	Facts       []string     `json:"facts"`
	Actions     []string     `json:"actions"`
	Limitations []string     `json:"limitations"`
	Created     string       `json:"created_at"`
}

func sourcePath(p string) bool {
	if len(p) > 512 || strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, segment := range strings.Split(p, "/") {
		if strings.HasPrefix(segment, ".") || segment == "node_modules" || segment == "vendor" || segment == "dist" || segment == "build" {
			return false
		}
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".cjs", ".mjs", ".py", ".java":
		return true
	}
	return false
}
func searchTerms(definition, result []byte) []string {
	var d struct {
		Steps []struct {
			Action string `json:"action"`
			Path   string `json:"path"`
		} `json:"steps"`
	}
	var report struct {
		Evidence struct {
			Requests []struct {
				Endpoint string `json:"endpoint"`
			} `json:"failed_requests"`
		} `json:"evidence"`
	}
	_ = json.Unmarshal(definition, &d)
	_ = json.Unmarshal(result, &report)
	terms := map[string]bool{}
	add := func(raw string) {
		u, e := url.Parse(raw)
		if e != nil {
			return
		}
		for _, part := range strings.Split(strings.ToLower(u.Path), "/") {
			if len(part) < 3 || len(part) > 32 {
				continue
			}
			safe := true
			for _, c := range part {
				if !(c >= 'a' && c <= 'z' || c == '-' || c == '_') {
					safe = false
				}
			}
			if safe {
				terms[part] = true
			}
		}
	}
	for _, s := range d.Steps {
		if s.Action == "navigate" {
			add(s.Path)
		}
	}
	for _, r := range report.Evidence.Requests {
		add(r.Endpoint)
	}
	if terms["login"] || terms["signin"] || terms["sign-in"] {
		terms["auth"] = true
		terms["login"] = true
	}
	out := []string{}
	for term := range terms {
		out = append(out, term)
	}
	sort.Strings(out)
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}
func rankSources(paths, terms []string, changed map[string]bool) []sourceFile {
	out := []sourceFile{}
	for _, p := range paths {
		if !sourcePath(p) {
			continue
		}
		lower := strings.ToLower(p)
		score := 0
		reasons := []string{}
		for _, term := range terms {
			if strings.Contains(lower, term) {
				score += 3
				reasons = append(reasons, "Path matches journey/request term: "+term)
			}
		}
		// Changed files alone are not enough to establish relevance.
		if score == 0 {
			continue
		}
		if changed[p] {
			score += 2
			reasons = append(reasons, "Changed since the earlier passing commit")
		}
		out = append(out, sourceFile{Path: p, Score: score, Changed: changed[p], Lines: []int{}, Reasons: reasons})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
func (h *Handler) Investigate(w http.ResponseWriter, r *http.Request) {
	user, ok := mw.GetUserID(r.Context())
	if !ok {
		fail(w, 401, "Authentication required.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	run, e := uuid.Parse(chi.URLParam(r, "runID"))
	if e != nil {
		fail(w, 400, "Invalid run.")
		return
	}
	var app, journey uuid.UUID
	var result, definition, release []byte
	var status string
	var queued time.Time
	e = h.DB.QueryRow(r.Context(), `SELECT j.application_id,j.id,r.status,r.definition,COALESCE(r.result,'{}'::jsonb),r.release_context,r.queued_at FROM browser_journey_runs r JOIN browser_journeys j ON j.id=r.journey_id JOIN applications a ON a.id=j.application_id WHERE r.id=$1 AND a.user_id=$2`, run, user).Scan(&app, &journey, &status, &definition, &result, &release, &queued)
	if e != nil {
		fail(w, 404, "Run not found.")
		return
	}
	if status != "failed" {
		fail(w, 409, "Source investigation requires a completed failed journey.")
		return
	}
	var contextData struct {
		SHA            string `json:"commit_sha"`
		Repository     string `json:"repository"`
		RepositoryID   int64  `json:"repository_id"`
		InstallationID int64  `json:"installation_id"`
	}
	if json.Unmarshal(release, &contextData) != nil || contextData.SHA == "" {
		fail(w, 409, "This run has no recorded commit. Queue a release run first.")
		return
	}
	verified, e := h.VerifyCommit(r, app, contextData.SHA)
	if e != nil {
		fail(w, 409, e.Error())
		return
	}
	if verified["repository_id"] != contextData.RepositoryID || verified["installation_id"] != contextData.InstallationID || verified["repository"] != contextData.Repository {
		fail(w, 409, "The application repository changed after this run. Reconnect the original repository to investigate.")
		return
	}
	_, token, e := h.token(r)
	if e != nil {
		fail(w, 409, "Reauthorize GitHub first.")
		return
	}
	parts := strings.Split(contextData.Repository, "/")
	if len(parts) != 2 {
		fail(w, 409, "Invalid recorded repository.")
		return
	}
	api := "https://api.github.com/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
	output := investigation{Commit: contextData.SHA, Repository: contextData.Repository, Candidates: []sourceFile{}, Facts: []string{"Commit existence and current repository access verified with GitHub."}, Actions: []string{}, Limitations: []string{"Static candidate search; it does not trace program execution or confirm a root cause.", "Preview-to-commit association was supplied by the operator.", "No repository code is executed or modified. At most five source files are inspected."}, Created: time.Now().UTC().Format(time.RFC3339)}
	var baseline string
	e = h.DB.QueryRow(ctx, `SELECT release_context->>'commit_sha' FROM browser_journey_runs WHERE journey_id=$1 AND status='passed' AND queued_at<$2 AND release_context->>'repository'=$3 AND release_context->>'repository_id'=$4 AND release_context->>'installation_id'=$5 AND release_context->>'commit_sha' ~ '^[0-9a-f]{40}$' ORDER BY queued_at DESC LIMIT 1`, journey, queued, contextData.Repository, fmt.Sprint(contextData.RepositoryID), fmt.Sprint(contextData.InstallationID)).Scan(&baseline)
	changed := map[string]bool{}
	if e == nil {
		output.Baseline = baseline
		if baseline == contextData.SHA {
			output.Facts = append(output.Facts, "Earlier passing run used the same commit; consider configuration, data, dependencies, or nondeterministic behavior.")
		} else {
			var comparison struct {
				Files []struct {
					Filename string `json:"filename"`
				} `json:"files"`
			}
			if h.request(ctx, api+"/compare/"+baseline+"..."+contextData.SHA, token, nil, &comparison) == nil {
				for _, f := range comparison.Files {
					changed[f.Filename] = true
				}
				output.Facts = append(output.Facts, fmt.Sprintf("GitHub comparison returned %d changed files.", len(comparison.Files)))
				output.Limitations = append(output.Limitations, "GitHub comparison file lists are bounded; large changes may be incomplete.")
			} else {
				output.Limitations = append(output.Limitations, "Baseline comparison unavailable; relevance ranking uses paths only.")
			}
		}
	} else {
		output.Facts = append(output.Facts, "No earlier passing release run exists for this journey and repository.")
	}
	var tree struct {
		Truncated bool `json:"truncated"`
		Tree      []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if h.request(ctx, api+"/git/trees/"+contextData.SHA+"?recursive=1", token, nil, &tree) != nil {
		fail(w, 502, "Source tree unavailable. Reauthorize or try a smaller repository.")
		return
	}
	if tree.Truncated {
		output.Limitations = append(output.Limitations, "GitHub truncated the source tree; some relevant files may be missing.")
	}
	paths := []string{}
	for _, item := range tree.Tree {
		if item.Type == "blob" {
			paths = append(paths, item.Path)
		}
	}
	terms := searchTerms(definition, result)
	output.Candidates = rankSources(paths, terms, changed)
	for i := range output.Candidates {
		file := &output.Candidates[i]
		escaped := []string{}
		for _, segment := range strings.Split(file.Path, "/") {
			escaped = append(escaped, url.PathEscape(segment))
		}
		file.URL = "https://github.com/" + contextData.Repository + "/blob/" + contextData.SHA + "/" + strings.Join(escaped, "/")
		var content struct {
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
			Size     int    `json:"size"`
			SHA      string `json:"sha"`
		}
		if h.request(ctx, api+"/contents/"+strings.Join(escaped, "/")+"?ref="+contextData.SHA, token, nil, &content) != nil || content.Encoding != "base64" || content.Size > 65536 {
			file.Reasons = append(file.Reasons, "Content unavailable or exceeds the 64 KB inspection limit")
			continue
		}
		source, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content.Content, "\n", ""))
		if err != nil || len(source) > 65536 {
			continue
		}
		for index, line := range strings.Split(string(source), "\n") {
			matched := false
			for _, term := range terms {
				if strings.Contains(strings.ToLower(line), term) {
					matched = true
					break
				}
			}
			if matched {
				file.Lines = append(file.Lines, index+1)
			}
			if len(file.Lines) >= 3 {
				break
			}
		}
		if len(file.Lines) > 0 {
			file.URL += fmt.Sprintf("#L%d", file.Lines[0])
		}
	}
	if len(output.Candidates) == 0 {
		output.Facts = append(output.Facts, "No supported source paths matched the journey/request terms.")
	}
	output.Actions = append(output.Actions, "Inspect the failed step and request evidence alongside the linked candidate files.", "Check environment variables, authentication/session behavior, and server logs at the run timestamp.", "Reproduce the failure and add a regression test before proposing a code change.")
	body, _ := json.Marshal(output)
	tag, e := h.DB.Exec(r.Context(), `UPDATE browser_journey_runs r SET source_investigation=$3::jsonb FROM browser_journeys j,applications a WHERE r.id=$1 AND r.journey_id=j.id AND j.application_id=a.id AND a.user_id=$2 AND r.status='failed'`, run, user, body)
	if e != nil {
		fail(w, 500, "Investigation could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "Run not found.")
		return
	}
	reply(w, 200, map[string]any{"investigation": output})
}
