package monitoring

import "strings"

func troubleshooting(message string) string {
	switch {
	case strings.Contains(message, "configuration"):
		return "Review the saved assertion settings and confirm the expected value is valid."

	case strings.Contains(message, "response text assertion"):
		return "Confirm this is the correct endpoint. Check whether the response format changed in the latest release. Inspect application logs for dependency failures."

	case strings.Contains(message, "not valid JSON"):
		return "Check whether the endpoint returned an HTML error or login page. Confirm the endpoint is intended to return JSON. Inspect server logs."

	case strings.Contains(message, "field was not found"):
		return "Compare the configured JSON Pointer with the current API response. Check for renamed fields or a changed response structure."

	case strings.Contains(message, "expected value"):
		return "Inspect the readiness field and the dependency it represents. Review application logs and recent configuration or deployment changes."

	case strings.Contains(message, "HTTP"):
		return "Inspect server logs at the check timestamp. Verify the endpoint, access requirements, and recent deployment changes. Consider a configured rollback if a release caused the failure."

	case strings.Contains(message, "timeout") || strings.Contains(message, "timed out"):
		return "Check service availability and resource usage. Inspect slow requests and dependency connection timeouts."

	case strings.Contains(message, "size limit"):
		return "Use a small health or readiness endpoint. Nimbus limits response bodies to 64 KB."

	case strings.Contains(message, "unsafe"):
		return "Use a public HTTP or HTTPS endpoint. Private network destinations are blocked."

	default:
		return "Check service availability, DNS, TLS certificates, and application logs at the check timestamp."
	}
}

func addTroubleshooting(result *CheckResult) {
	if result.ErrorMessage == nil {
		return
	}
	message := *result.ErrorMessage
	message += " Suggested next steps (possible causes): " + troubleshooting(message)
	result.ErrorMessage = &message
}
