package aws

import "testing"

func TestSafeShellValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"simple profile", "zenith-dev", true},
		{"with dots", "eu-west-2", true},
		{"with slash", "path/to/thing", true},
		{"with underscore", "my_profile", true},
		{"with space", "my profile", false},
		{"with semicolon", "a;b", false},
		{"with backtick", "a`b", false},
		{"with dollar", "a$b", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeShellValue.MatchString(tt.input)
			if result != tt.isValid {
				t.Errorf("safeShellValue.MatchString(%q) = %v, want %v", tt.input, result, tt.isValid)
			}
		})
	}
}
