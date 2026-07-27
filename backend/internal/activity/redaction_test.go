package activity

import (
	"strings"
	"testing"
)

func TestSanitizeMetadataRedactsSensitiveValues(
	t *testing.T,
) {
	t.Parallel()

	result := SanitizeMetadata(
		map[string]any{
			"version":       "v1.2.3",
			"password":      "super-secret",
			"webhook_token": "token-value",
			"nested": map[string]any{
				"authorization": "Bearer abc.def.ghi",
				"safe_value":    "visible",
			},
		},
	).(map[string]any)

	if result["version"] != "v1.2.3" {
		t.Fatal("safe version value was changed")
	}

	if result["password"] != redactedValue {
		t.Fatal("password was not redacted")
	}

	if result["webhook_token"] != redactedValue {
		t.Fatal("webhook token was not redacted")
	}

	nested, ok := result["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested metadata was not preserved")
	}

	if nested["authorization"] != redactedValue {
		t.Fatal("authorization was not redacted")
	}

	if nested["safe_value"] != "visible" {
		t.Fatal("safe nested value was changed")
	}
}

func TestRedactTextRemovesBearerTokens(
	t *testing.T,
) {
	t.Parallel()

	result := RedactText(
		"request failed with Authorization: Bearer abc.def.ghi",
	)

	if strings.Contains(result, "abc.def.ghi") {
		t.Fatal("bearer token remained in text")
	}

	if !strings.Contains(result, redactedValue) {
		t.Fatal("redaction marker was not added")
	}
}

func TestRedactTextRemovesJWTs(
	t *testing.T,
) {
	t.Parallel()

	value := "eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoiMTIzIn0.signature"

	result := RedactText(
		"token was " + value,
	)

	if strings.Contains(result, value) {
		t.Fatal("JWT remained in text")
	}
}
