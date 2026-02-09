package utils

import "testing"

func TestIsProductionEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected bool
	}{
		{"prod", "prod", true},
		{"preprod", "preprod", true},
		{"trg", "trg", true},
		{"live", "live", true},
		{"dev", "dev", false},
		{"sit", "sit", false},
		{"qa", "qa", false},
		{"snd", "snd", false},
		{"case insensitive", "PROD", true},
		{"case insensitive preprod", "PreProd", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProductionEnvironment(tt.env)
			if result != tt.expected {
				t.Errorf("IsProductionEnvironment(%q) = %v, want %v", tt.env, result, tt.expected)
			}
		})
	}
}
