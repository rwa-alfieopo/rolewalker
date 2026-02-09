package aws

import "testing"

func TestBuildHPAName(t *testing.T) {
	sm := &ScalingManager{namespace: "zenith"}

	tests := []struct {
		name     string
		service  string
		expected string
	}{
		{"already has hpa suffix", "candidate-microservice-hpa", "candidate-microservice-hpa"},
		{"has microservice suffix", "candidate-microservice", "candidate-microservice-hpa"},
		{"bare service name", "candidate", "candidate-microservice-hpa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sm.buildHPAName(tt.service)
			if result != tt.expected {
				t.Errorf("buildHPAName(%q) = %q, want %q", tt.service, result, tt.expected)
			}
		})
	}
}

func TestScalingManagerValidEnvironments(t *testing.T) {
	sm := &ScalingManager{configRepo: nil}
	envs := sm.ValidEnvironments()
	if len(envs) == 0 {
		t.Error("ValidEnvironments() returned empty list")
	}
	// Should return defaults when no DB
	found := false
	for _, e := range envs {
		if e == "dev" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ValidEnvironments() should contain 'dev' in defaults")
	}
}

func TestScalingManagerValidPresets(t *testing.T) {
	sm := &ScalingManager{configRepo: nil}
	presets := sm.ValidPresets()
	if len(presets) == 0 {
		t.Error("ValidPresets() returned empty list")
	}
	found := false
	for _, p := range presets {
		if p == "normal" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ValidPresets() should contain 'normal' in defaults")
	}
}

func TestScalingManagerIsValidEnv(t *testing.T) {
	sm := &ScalingManager{configRepo: nil}

	if !sm.isValidEnv("dev") {
		t.Error("isValidEnv('dev') should be true")
	}
	if sm.isValidEnv("nonexistent") {
		t.Error("isValidEnv('nonexistent') should be false")
	}
}
