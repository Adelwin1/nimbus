package incident

import "testing"

func TestDetermineApplicationStatus(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name        string
		observation storedHealthObservation
		expected    string
	}{
		{
			name: "healthy application",
			observation: storedHealthObservation{
				Healthy:            true,
				LatencyMS:          100,
				LatencyThresholdMS: 1000,
			},
			expected: "healthy",
		},
		{
			name: "slow application",
			observation: storedHealthObservation{
				Healthy:            true,
				LatencyMS:          1500,
				LatencyThresholdMS: 1000,
			},
			expected: "degraded",
		},
		{
			name: "failure below threshold",
			observation: storedHealthObservation{
				Healthy:             false,
				ConsecutiveFailures: 2,
				FailureThreshold:    3,
			},
			expected: "degraded",
		},
		{
			name: "failure reaches threshold",
			observation: storedHealthObservation{
				Healthy:             false,
				ConsecutiveFailures: 3,
				FailureThreshold:    3,
			},
			expected: "down",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := determineApplicationStatus(
				test.observation,
			)

			if actual != test.expected {
				t.Fatalf(
					"status = %s, expected %s",
					actual,
					test.expected,
				)
			}
		})
	}
}
