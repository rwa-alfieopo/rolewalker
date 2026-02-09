package aws

import "testing"

func TestExtractEnvName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"zenith prefix", "zenith-dev", "dev"},
		{"zenith prod", "zenith-prod", "prod"},
		{"aws prefix", "aws-staging", "staging"},
		{"no prefix", "dev", "dev"},
		{"compound name", "zenith-dev-admin", "dev"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEnvName(tt.input)
			if result != tt.expected {
				t.Errorf("extractEnvName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
