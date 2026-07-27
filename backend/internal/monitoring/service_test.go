package monitoring

import "testing"

func TestEvaluateApplicationState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		result           CheckResult
		currentFailures  int
		failureThreshold int
		expectedStatus   string
		expectedFailures int
		expectedHealthy  bool
	}{
		{
			name: "successful fast check",
			result: CheckResult{
				Healthy: true,
				Slow:    false,
			},
			currentFailures:  2,
			failureThreshold: 3,
			expectedStatus:   "healthy",
			expectedFailures: 0,
			expectedHealthy:  true,
		},
		{
			name: "successful slow check",
			result: CheckResult{
				Healthy: true,
				Slow:    true,
			},
			currentFailures:  2,
			failureThreshold: 3,
			expectedStatus:   "degraded",
			expectedFailures: 0,
			expectedHealthy:  false,
		},
		{
			name: "failure below threshold",
			result: CheckResult{
				Healthy: false,
			},
			currentFailures:  0,
			failureThreshold: 3,
			expectedStatus:   "degraded",
			expectedFailures: 1,
			expectedHealthy:  false,
		},
		{
			name: "failure reaches threshold",
			result: CheckResult{
				Healthy: false,
			},
			currentFailures:  2,
			failureThreshold: 3,
			expectedStatus:   "down",
			expectedFailures: 3,
			expectedHealthy:  false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			status, failures, healthy :=
				evaluateApplicationState(
					test.result,
					test.currentFailures,
					test.failureThreshold,
				)

			if status != test.expectedStatus {
				t.Fatalf(
					"status = %s, expected %s",
					status,
					test.expectedStatus,
				)
			}

			if failures != test.expectedFailures {
				t.Fatalf(
					"failures = %d, expected %d",
					failures,
					test.expectedFailures,
				)
			}

			if healthy != test.expectedHealthy {
				t.Fatalf(
					"markHealthy = %v, expected %v",
					healthy,
					test.expectedHealthy,
				)
			}
		})
	}
}
