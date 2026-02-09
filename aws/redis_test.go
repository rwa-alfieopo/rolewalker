package aws

import "testing"

func TestParseRedisHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"with port", "redis.example.com:6379", "redis.example.com"},
		{"without port", "redis.example.com", "redis.example.com"},
		{"with non-numeric suffix", "redis.example.com:abc", "redis.example.com:abc"},
		{"empty", "", ""},
		{"just port", ":6379", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRedisHost(tt.input)
			if result != tt.expected {
				t.Errorf("parseRedisHost(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
