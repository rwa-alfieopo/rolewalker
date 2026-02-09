package aws

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateTunnelID(t *testing.T) {
	tests := []struct {
		service  string
		env      string
		expected string
	}{
		{"db", "dev", "db-dev"},
		{"redis", "prod", "redis-prod"},
		{"msk", "qa", "msk-qa"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := GenerateTunnelID(tt.service, tt.env)
			if result != tt.expected {
				t.Errorf("GenerateTunnelID(%q, %q) = %q, want %q", tt.service, tt.env, result, tt.expected)
			}
		})
	}
}

func TestTunnelStateSerialization(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "tunnels.json")

	ts := &TunnelState{
		Tunnels:  make(map[string]*TunnelInfo),
		filePath: statePath,
	}

	tunnel := &TunnelInfo{
		ID:          "db-dev",
		Service:     "db",
		Environment: "dev",
		PodName:     "test-pod",
		LocalPort:   5432,
		RemoteHost:  "db.example.com",
		RemotePort:  5432,
		StartedAt:   time.Now(),
	}

	// Test Add
	if err := ts.Add(tunnel); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	var loaded TunnelState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal state: %v", err)
	}

	if len(loaded.Tunnels) != 1 {
		t.Errorf("Expected 1 tunnel, got %d", len(loaded.Tunnels))
	}

	// Test Get
	got := ts.Get("db-dev")
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.PodName != "test-pod" {
		t.Errorf("Get().PodName = %q, want %q", got.PodName, "test-pod")
	}

	// Test GetByServiceEnv
	got = ts.GetByServiceEnv("db", "dev")
	if got == nil {
		t.Fatal("GetByServiceEnv() returned nil")
	}

	// Test List
	list := ts.List()
	if len(list) != 1 {
		t.Errorf("List() length = %d, want 1", len(list))
	}

	// Test Remove
	if err := ts.Remove("db-dev"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if ts.Get("db-dev") != nil {
		t.Error("Get() should return nil after Remove()")
	}

	// Test Clear
	ts.Add(tunnel)
	if err := ts.Clear(); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}
	if len(ts.List()) != 0 {
		t.Error("List() should be empty after Clear()")
	}
}
