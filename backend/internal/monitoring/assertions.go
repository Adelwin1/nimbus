package monitoring

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

type CheckRules struct {
	ExpectedStatus *int            `json:"expected_status"`
	RequiredText   string          `json:"required_text"`
	JSONPointer    *string         `json:"json_pointer"`
	JSONExpected   json.RawMessage `json:"json_expected"`
}

var pointerPattern = regexp.MustCompile(`^(/([^~]|~[01])*)*$`)

func evaluateResponse(status int, body []byte, rules CheckRules) string {
	if rules.ExpectedStatus != nil {
		if *rules.ExpectedStatus < 100 || *rules.ExpectedStatus > 599 {
			return "invalid expected HTTP status configuration"
		}
		if status != *rules.ExpectedStatus {
			return "HTTP status assertion failed: expected " +
				strconv.Itoa(*rules.ExpectedStatus) + ", received " + strconv.Itoa(status)
		}
	} else if status < 200 || status >= 300 {
		return "health endpoint returned HTTP " + strconv.Itoa(status)
	}

	if rules.RequiredText != "" && !bytes.Contains(body, []byte(rules.RequiredText)) {
		return "response text assertion failed: required text was not found"
	}

	if rules.JSONPointer == nil {
		if len(rules.JSONExpected) > 0 {
			return "invalid JSON assertion configuration"
		}
		return ""
	}
	if !pointerPattern.MatchString(*rules.JSONPointer) || len(*rules.JSONPointer) > 512 {
		return "invalid JSON pointer configuration"
	}

	expected, ok := decodeAssertionJSON(rules.JSONExpected)
	if !ok {
		return "invalid expected JSON value configuration"
	}
	actual, ok := decodeAssertionJSON(body)
	if !ok {
		return "JSON assertion failed: response is not valid JSON"
	}
	selected, ok := selectJSON(actual, *rules.JSONPointer)
	if !ok {
		return "JSON assertion failed: configured field was not found"
	}
	if !equalJSON(selected, expected) {
		return "JSON assertion failed: field did not match the expected value"
	}
	return ""
}

func decodeAssertionJSON(data []byte) (any, bool) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value any
	if d.Decode(&value) != nil {
		return nil, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, false
	}
	return value, true
}

func selectJSON(value any, pointer string) (any, bool) {
	if pointer == "" {
		return value, true
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[key]
			if !ok {
				return nil, false
			}
		case []any:
			if key == "" || (len(key) > 1 && key[0] == '0') {
				return nil, false
			}
			for _, c := range key {
				if c < '0' || c > '9' {
					return nil, false
				}
			}
			index, err := strconv.Atoi(key)
			if err != nil || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func equalJSON(a, b any) bool {
	switch left := a.(type) {
	case nil:
		return b == nil
	case bool:
		right, ok := b.(bool)
		return ok && left == right
	case string:
		right, ok := b.(string)
		return ok && left == right
	case json.Number:
		right, ok := b.(json.Number)
		if !ok || !boundedNumber(string(left)) || !boundedNumber(string(right)) {
			return false
		}
		x, ok := new(big.Rat).SetString(string(left))
		if !ok {
			return false
		}
		y, ok := new(big.Rat).SetString(string(right))
		return ok && x.Cmp(y) == 0
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i := range left {
			if !equalJSON(left[i], right[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !equalJSON(value, other) {
				return false
			}
		}
		return true
	}
	return false
}

func boundedNumber(value string) bool {
	if len(value) > 4096 {
		return false
	}
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		exponent, err := strconv.Atoi(value[index+1:])
		return err == nil && exponent >= -10000 && exponent <= 10000
	}
	return true
}
