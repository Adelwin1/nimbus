package activity

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPAddressWithPort(
	t *testing.T,
) {
	t.Parallel()

	request := httptest.NewRequest(
		"GET",
		"http://example.com",
		nil,
	)
	request.RemoteAddr = "203.0.113.10:45678"

	actual := clientIPAddress(request)

	if actual != "203.0.113.10" {
		t.Fatalf(
			"clientIPAddress() = %q, expected %q",
			actual,
			"203.0.113.10",
		)
	}
}

func TestClientIPAddressWithoutPort(
	t *testing.T,
) {
	t.Parallel()

	request := httptest.NewRequest(
		"GET",
		"http://example.com",
		nil,
	)
	request.RemoteAddr = "2001:db8::10"

	actual := clientIPAddress(request)

	if actual != "2001:db8::10" {
		t.Fatalf(
			"clientIPAddress() = %q, expected %q",
			actual,
			"2001:db8::10",
		)
	}
}

func TestClientIPAddressRejectsInvalidValue(
	t *testing.T,
) {
	t.Parallel()

	request := httptest.NewRequest(
		"GET",
		"http://example.com",
		nil,
	)
	request.RemoteAddr = "not-an-ip-address"

	if actual := clientIPAddress(request); actual != "" {
		t.Fatalf(
			"clientIPAddress() = %q, expected empty value",
			actual,
		)
	}
}
