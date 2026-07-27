package activity

import (
	"encoding/json"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var (
	bearerTokenPattern = regexp.MustCompile(
		`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`,
	)

	jwtPattern = regexp.MustCompile(
		`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`,
	)
)

func SanitizeMetadata(value any) any {
	if value == nil {
		return map[string]any{}
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{
			"metadata": redactedValue,
		}
	}

	var generic any

	if err := json.Unmarshal(encoded, &generic); err != nil {
		return map[string]any{
			"metadata": redactedValue,
		}
	}

	return sanitizeValue(generic)
}

func RedactText(value string) string {
	value = bearerTokenPattern.ReplaceAllString(
		value,
		"Bearer "+redactedValue,
	)

	value = jwtPattern.ReplaceAllString(
		value,
		redactedValue,
	)

	return value
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(
			map[string]any,
			len(typed),
		)

		for key, item := range typed {
			if isSensitiveKey(key) {
				result[key] = redactedValue
				continue
			}

			result[key] = sanitizeValue(item)
		}

		return result

	case []any:
		result := make([]any, len(typed))

		for index, item := range typed {
			result[index] = sanitizeValue(item)
		}

		return result

	case string:
		return RedactText(typed)

	default:
		return typed
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(
		strings.TrimSpace(key),
	)

	normalized = strings.NewReplacer(
		"-",
		"_",
		" ",
		"_",
		".",
		"_",
	).Replace(normalized)

	sensitiveTerms := []string{
		"password",
		"secret",
		"token",
		"authorization",
		"api_key",
		"apikey",
		"private_key",
		"encryption_key",
		"webhook_url",
		"cookie",
		"session",
	}

	for _, term := range sensitiveTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}

	return false
}
