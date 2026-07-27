package testutil

import "testing"

func TestIsSafeTestDatabaseName(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name     string
		database string
		expected bool
	}{
		{
			name:     "standard test database",
			database: "nimbus_test",
			expected: true,
		},
		{
			name:     "test prefix",
			database: "test_nimbus",
			expected: true,
		},
		{
			name:     "integration test database",
			database: "nimbus_integration_test",
			expected: true,
		},
		{
			name:     "development database",
			database: "nimbus",
			expected: false,
		},
		{
			name:     "postgres database",
			database: "postgres",
			expected: false,
		},
		{
			name:     "empty database",
			database: "",
			expected: false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()

				actual :=
					IsSafeTestDatabaseName(
						test.database,
					)

				if actual != test.expected {
					t.Fatalf(
						"IsSafeTestDatabaseName(%q) = %v, expected %v",
						test.database,
						actual,
						test.expected,
					)
				}
			},
		)
	}
}
