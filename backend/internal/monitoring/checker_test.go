package monitoring

import (
	"errors"
	"net"
	"net/url"
	"testing"
)

func TestValidateTargetURLRejectsUnsafeURLs(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://localhost:8080/health",
		"http://service.localhost/health",
		"http://127.0.0.1/health",
		"http://10.0.0.5/health",
		"http://169.254.169.254/latest/meta-data",
		"http://192.168.1.20/health",
		"http://[::1]/health",
		"file:///etc/passwd",
	}

	for _, value := range tests {
		value := value

		t.Run(value, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(value)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}

			err = validateTargetURL(parsed)
			if err == nil {
				t.Fatalf("expected unsafe URL to be rejected")
			}
		})
	}
}

func TestValidateTargetURLAcceptsPublicHTTPURLs(t *testing.T) {
	t.Parallel()

	tests := []string{
		"https://example.com/health",
		"https://api.github.com",
		"http://8.8.8.8/health",
	}

	for _, value := range tests {
		value := value

		t.Run(value, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(value)
			if err != nil {
				t.Fatalf("parse URL: %v", err)
			}

			if err := validateTargetURL(parsed); err != nil {
				t.Fatalf("expected URL to be accepted: %v", err)
			}
		})
	}
}

func TestIsSafePublicIP(t *testing.T) {
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

			actual := isSafePublicIP(net.ParseIP(test.ip))
			if actual != test.expected {
				t.Fatalf(
					"isSafePublicIP(%s) = %v, expected %v",
					test.ip,
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestValidateTargetURLRejectsCredentials(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(
		"https://username:password@example.com/health",
	)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	err = validateTargetURL(parsed)
	if err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}

	if errors.Is(err, ErrUnsafeDestination) {
		t.Fatal("expected a credential validation error")
	}
}
