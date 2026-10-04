package monitoring

import (
	"strings"
	"testing"
)

func TestTroubleshootingPreservesFailure(t *testing.T) {
	message := "JSON assertion failed: configured field was not found"
	result := CheckResult{ErrorMessage: &message}
	addTroubleshooting(&result)

	if result.Healthy ||
		!strings.HasPrefix(*result.ErrorMessage, message) ||
		!strings.Contains(*result.ErrorMessage, "possible causes") ||
		!strings.Contains(*result.ErrorMessage, "JSON Pointer") {
		t.Fatal("expected failure evidence and qualified troubleshooting")
	}

	healthy := CheckResult{Healthy: true}
	addTroubleshooting(&healthy)
	if healthy.ErrorMessage != nil {
		t.Fatal("healthy checks must not acquire errors")
	}
}
