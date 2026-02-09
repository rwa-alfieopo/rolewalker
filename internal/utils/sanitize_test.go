package utils

import "testing"

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "johndoe", "johndoe"},
		{"with dots", "john.doe", "johndoe"},
		{"with at", "john@example.com", "johnexamplecom"},
		{"with dashes", "john-doe", "john-doe"},
		{"with spaces", "john doe", "johndoe"},
		{"empty", "", ""},
		{"long name truncated", "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrst"},
		{"exactly 20", "abcdefghijklmnopqrst", "abcdefghijklmnopqrst"},
		{"special chars", "user!@#$%^&*()", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeUsername(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeUsername(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "value", "value"},
		{"with at", "user@example.com", "useratexample.com"},
		{"with spaces", "hello world", "hello-world"},
		{"with dots", "v1.2.3", "v1.2.3"},
		{"with underscores", "my_label", "my_label"},
		{"with dashes", "my-label", "my-label"},
		{"empty", "", ""},
		{"long value truncated", string(make([]byte, 100)), string(make([]byte, 63))},
		{"special chars", "a!b#c$d", "a-b-c-d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeLabelValue(tt.input)
			if len(result) > 63 {
				t.Errorf("SanitizeLabelValue(%q) length = %d, want <= 63", tt.input, len(result))
			}
			if tt.name != "long value truncated" && result != tt.expected {
				t.Errorf("SanitizeLabelValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetCurrentUsername(t *testing.T) {
	result := GetCurrentUsername()
	if result == "" {
		t.Error("GetCurrentUsername() returned empty string")
	}
}

func TestGetCurrentUsernamePodSafe(t *testing.T) {
	result := GetCurrentUsernamePodSafe()
	if result == "" {
		t.Error("GetCurrentUsernamePodSafe() returned empty string")
	}
	if len(result) > 20 {
		t.Errorf("GetCurrentUsernamePodSafe() length = %d, want <= 20", len(result))
	}
}
