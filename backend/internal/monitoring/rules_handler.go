package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func rulesReply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func rulesError(w http.ResponseWriter, status int, message string) {
	rulesReply(w, status, map[string]any{
		"error": map[string]string{"message": message},
	})
}

func validateRules(rules *CheckRules) string {
	if rules.ExpectedStatus != nil &&
		(*rules.ExpectedStatus < 100 || *rules.ExpectedStatus > 599) {
		return "Expected status must be between 100 and 599."
	}
	if !utf8.ValidString(rules.RequiredText) ||
		utf8.RuneCountInString(rules.RequiredText) > 2048 {
		return "Required text must contain at most 2,048 characters."
	}
	if rules.JSONPointer == nil {
		if len(rules.JSONExpected) > 0 &&
			!bytes.Equal(bytes.TrimSpace(rules.JSONExpected), []byte("null")) {
			return "Select a JSON field for the expected value."
		}
		rules.JSONExpected = nil
		return ""
	}
	if !utf8.ValidString(*rules.JSONPointer) ||
		len(*rules.JSONPointer) > 512 ||
		!pointerPattern.MatchString(*rules.JSONPointer) {
		return "Use a valid JSON Pointer, such as /database/connected, of at most 512 bytes."
	}
	if len(rules.JSONExpected) > 4096 {
		return "Expected JSON must be at most 4 KB."
	}
	if _, ok := decodeAssertionJSON(rules.JSONExpected); !ok {
		return "Enter a valid expected JSON value."
	}
	return ""
}

func (h *Handler) CheckRulesSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}
	app, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		rulesError(w, 400, "Invalid application ID.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if r.Method == "GET" {
		if _, err := h.service.repository.GetCheckTarget(ctx, app, userID); err != nil {
			if err == ErrApplicationNotFound {
				rulesError(w, 404, "Application not found.")
			} else {
				rulesError(w, 500, "Check settings unavailable.")
			}
			return
		}
		rules, err := h.service.repository.LoadCheckRules(ctx, app)
		if err != nil {
			rulesError(w, 500, "Check settings unavailable.")
			return
		}
		rulesReply(w, 200, rules)
		return
	}

	var rules CheckRules
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	d.DisallowUnknownFields()
	err = d.Decode(&rules)
	var extra any
	if err != nil || d.Decode(&extra) != io.EOF {
		rulesError(w, 400, "Invalid check settings.")
		return
	}
	if message := validateRules(&rules); message != "" {
		rulesError(w, 400, message)
		return
	}

	tag, err := h.service.repository.db.Exec(ctx, `
 INSERT INTO application_check_rules(
  application_id,expected_status,required_text,json_pointer,json_expected
 )
 SELECT id,$3,$4,$5,$6::jsonb FROM applications WHERE id=$1 AND user_id=$2
 ON CONFLICT(application_id) DO UPDATE SET
 expected_status=EXCLUDED.expected_status,
 required_text=EXCLUDED.required_text,
 json_pointer=EXCLUDED.json_pointer,
 json_expected=EXCLUDED.json_expected,
 updated_at=now()`,
		app, userID, rules.ExpectedStatus, rules.RequiredText,
		rules.JSONPointer, []byte(rules.JSONExpected))

	if err != nil {
		rulesError(w, 500, "Check settings could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		rulesError(w, 404, "Application not found.")
		return
	}
	rulesReply(w, 200, rules)
}
