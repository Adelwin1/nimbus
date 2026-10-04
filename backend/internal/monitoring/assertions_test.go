package monitoring

import (
	"encoding/json"
	"testing"
)

func TestResponseAssertions(t *testing.T) {
	status := 200
	pointer := "/database/connected"
	root := ""
	rules := CheckRules{
		JSONPointer:  &pointer,
		JSONExpected: json.RawMessage(`true`),
	}
	tests := []struct {
		name   string
		status int
		body   string
		rules  CheckRules
		pass   bool
	}{
		{"legacy success", 204, "", CheckRules{}, true},
		{"legacy failure", 500, "", CheckRules{}, false},
		{"expected status", 200, "", CheckRules{ExpectedStatus: &status}, true},
		{"wrong status", 201, "", CheckRules{ExpectedStatus: &status}, false},
		{"required text", 200, "database connected", CheckRules{RequiredText: "connected"}, true},
		{"missing text", 200, "unavailable", CheckRules{RequiredText: "connected"}, false},
		{"JSON match", 200, `{"database":{"connected":true}}`, rules, true},
		{"JSON mismatch", 200, `{"database":{"connected":false}}`, rules, false},
		{"missing field", 200, `{}`, rules, false},
		{"invalid JSON", 200, `not json`, rules, false},
		{"precise numbers", 200, `9007199254740993`,
			CheckRules{JSONPointer: &root, JSONExpected: json.RawMessage(`9007199254740992`)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := evaluateResponse(tt.status, []byte(tt.body), tt.rules)
			if (message == "") != tt.pass {
				t.Fatalf("expected pass=%v; failure=%q", tt.pass, message)
			}
		})
	}
}
