package deployment

import (
	"net"
	"net/url"
	"testing"
)

func TestValidateWebhookURLRejectsUnsafeDestinations(
	t *testing.T,
) {
	t.Parallel()

	tests := []string{
		"http://localhost:8080/deploy",
		"http://service.localhost/deploy",
		"http://127.0.0.1/deploy",
		"http://10.0.0.5/deploy",
		"http://169.254.169.254/latest/meta-data",
		"http://192.168.1.10/deploy",
		"http://[::1]/deploy",
		"file:///etc/passwd",
	}

	for _, value := range tests {
		value := value

		t.Run(value, func(t *testing.T) {
			t.Parallel()

			parsedURL, err := url.Parse(value)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}

			if err := validateWebhookURL(parsedURL); err == nil {
				t.Fatal(
					"expected unsafe webhook URL to be rejected",
				)
			}
		})
	}
}

func TestValidateWebhookURLAcceptsPublicURLs(
	t *testing.T,
) {
	t.Parallel()

	tests := []string{
		"https://example.com/hooks/deploy",
		"https://api.github.com/repos/example/deployments",
		"http://8.8.8.8/deploy",
	}

	for _, value := range tests {
		value := value

		t.Run(value, func(t *testing.T) {
			t.Parallel()

			parsedURL, err := url.Parse(value)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}

			if err := validateWebhookURL(parsedURL); err != nil {
				t.Fatalf(
					"expected URL to be accepted: %v",
					err,
				)
			}
		})
	}
}

func TestIsSafeWebhookIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ip       string
		expected bool
	}{
		{ip: "8.8.8.8", expected: true},
		{ip: "1.1.1.1", expected: true},
		{ip: "127.0.0.1", expected: false},
		{ip: "10.0.0.1", expected: false},
		{ip: "172.16.0.1", expected: false},
		{ip: "192.168.1.1", expected: false},
		{ip: "169.254.169.254", expected: false},
		{ip: "::1", expected: false},
		{ip: "fc00::1", expected: false},
	}

	for _, test := range tests {
		test := test

		t.Run(test.ip, func(t *testing.T) {
			t.Parallel()

			actual := isSafeWebhookIP(
				net.ParseIP(test.ip),
			)

			if actual != test.expected {
				t.Fatalf(
					"isSafeWebhookIP(%s) = %v, expected %v",
					test.ip,
					actual,
					test.expected,
				)
			}
		})
	}
}
