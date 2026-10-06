package journeys

import (
	"strings"
	"testing"
)

func TestReleaseInput(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct {
		name  string
		input RunInput
		want  bool
	}{
		{"legacy", RunInput{}, true},
		{"public preview", RunInput{sha, "https://preview.example.com"}, true},
		{"missing commit", RunInput{"", "https://preview.example.com"}, false},
		{"short commit", RunInput{"abc", "https://preview.example.com"}, false},
		{"http", RunInput{sha, "http://preview.example.com"}, false},
		{"credentials", RunInput{sha, "https://user:secret@preview.example.com"}, false},
		{"query", RunInput{sha, "https://preview.example.com?token=secret"}, false},
		{"localhost", RunInput{sha, "https://localhost"}, false},
		{"private IP", RunInput{sha, "https://192.168.1.1"}, false},
		{"path", RunInput{sha, "https://preview.example.com/login"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validRelease(&tc.input); got != tc.want {
				t.Fatalf("valid=%v want=%v", got, tc.want)
			}
		})
	}
}
