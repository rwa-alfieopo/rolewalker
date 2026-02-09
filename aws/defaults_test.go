package aws

import "testing"

func TestDefaultEnvironmentsNotEmpty(t *testing.T) {
	if len(DefaultEnvironments) == 0 {
		t.Error("DefaultEnvironments should not be empty")
	}
}

func TestDefaultPresetsNotEmpty(t *testing.T) {
	if len(DefaultPresets) == 0 {
		t.Error("DefaultPresets should not be empty")
	}
}

func TestDefaultServicesNotEmpty(t *testing.T) {
	if DefaultServices == "" {
		t.Error("DefaultServices should not be empty")
	}
}

func TestDefaultGRPCServicesNotEmpty(t *testing.T) {
	if DefaultGRPCServices == "" {
		t.Error("DefaultGRPCServices should not be empty")
	}
}
