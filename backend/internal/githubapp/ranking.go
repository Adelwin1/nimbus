package githubapp

import (
	"encoding/json"
	"net/url"
	"path"
	"sort"
	"strings"
)

type failureLocation struct {
	Route    string
	Action   string
	APIError bool
}

func locateFailure(definition, result []byte) failureLocation {
	var d struct {
		Steps []struct {
			Action string `json:"action"`
			Path   string `json:"path"`
		} `json:"steps"`
	}
	var report struct {
		Steps []struct {
			Number int    `json:"number"`
			Action string `json:"action"`
			Passed bool   `json:"passed"`
		} `json:"steps"`
		Evidence struct {
			Requests []struct {
				Endpoint string `json:"endpoint"`
				Status   *int   `json:"status"`
				Resource string `json:"resource_type"`
			} `json:"failed_requests"`
		} `json:"evidence"`
	}
	_ = json.Unmarshal(definition, &d)
	_ = json.Unmarshal(result, &report)
	location := failureLocation{}
	failed := len(d.Steps)
	for _, step := range report.Steps {
		if !step.Passed {
			failed = step.Number
			location.Action = step.Action
			break
		}
	}
	if failed < 1 || failed > len(d.Steps) {
		return location
	}
	for _, step := range d.Steps[:failed] {
		if step.Action == "navigate" {
			u, e := url.Parse(step.Path)
			if e == nil {
				location.Route = strings.Trim(u.Path, "/")
			}
		}
	}
	for _, request := range report.Evidence.Requests {
		// Only failed API-like requests justify a backend preference; missing images do not.
		u, e := url.Parse(request.Endpoint)
		if e != nil {
			continue
		}
		apiLike := request.Resource == "fetch" || request.Resource == "xhr" || strings.HasPrefix(u.Path, "/api/")
		if apiLike && (request.Status == nil || *request.Status >= 400) {
			location.APIError = true
		}
	}
	return location
}
func rankForFailure(paths, terms []string, changed map[string]bool, definition, result []byte) []sourceFile {
	location := locateFailure(definition, result)
	candidates := []sourceFile{}
	for _, p := range paths {
		// Rank before applying the five-file limit so frontend candidates aren't lost to ties.
		items := rankSources([]string{p}, terms, changed)
		if len(items) == 0 {
			continue
		}
		item := items[0]
		lower := strings.ToLower(p)
		frontend := strings.HasPrefix(lower, "frontend/") || strings.HasPrefix(lower, "app/") || strings.HasPrefix(lower, "src/app/") || strings.HasPrefix(lower, "pages/") || strings.Contains(lower, "/components/")
		backend := strings.HasPrefix(lower, "backend/") || strings.Contains(lower, "/server/") || strings.Contains(lower, "/api/")
		uiFailure := location.Action == "expect_visible" || location.Action == "expect_text" || location.Action == "click" || location.Action == "fill"
		if uiFailure && !location.APIError && frontend {
			item.Score += 8
			item.Reasons = append(item.Reasons, "Element/assertion failed without failed API-request evidence; prefer the UI source")
		}
		route := strings.ToLower(location.Route)
		if route != "" && (strings.Contains(lower, "/app/"+route+"/page.") || strings.Contains(lower, "/pages/"+route+".") || strings.Contains(lower, "/pages/"+route+"/index.")) {
			item.Score += 12
			item.Reasons = append(item.Reasons, "Page path matches navigation before the failed step")
		}
		if location.APIError && backend {
			item.Score += 12
			item.Reasons = append(item.Reasons, "Failed API-like request evidence; prefer backend handlers and services")
		}
		isTest := strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") || strings.Contains(lower, "/__tests__/")
		if isTest {
			item.Score -= 4
			item.Reasons = append(item.Reasons, "Test file is supporting context rather than the primary implementation")
		}
		if path.Base(lower) == "model.go" {
			item.Score--
		}
		candidates = append(candidates, item)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return candidates
}
