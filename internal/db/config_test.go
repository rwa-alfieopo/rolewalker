package db

import (
	"testing"
)

func TestNewDB(t *testing.T) {
	database, err := NewDB()
	if err != nil {
		t.Fatalf("NewDB() error: %v", err)
	}
	defer database.Close()

	// Verify the database is usable
	if err := database.Ping(); err != nil {
		t.Fatalf("Ping() error: %v", err)
	}
}

func TestConfigRepository_GetEnvironment_NotFound(t *testing.T) {
	database, err := NewDB()
	if err != nil {
		t.Fatalf("NewDB() error: %v", err)
	}
	defer database.Close()

	repo := NewConfigRepository(database)
	_, err = repo.GetEnvironment("nonexistent-env-xyz")
	if err == nil {
		t.Error("GetEnvironment() should return error for nonexistent environment")
	}
}

func TestConfigRepository_GetAllEnvironments(t *testing.T) {
	database, err := NewDB()
	if err != nil {
		t.Fatalf("NewDB() error: %v", err)
	}
	defer database.Close()

	repo := NewConfigRepository(database)
	envs, err := repo.GetAllEnvironments()
	if err != nil {
		t.Fatalf("GetAllEnvironments() error: %v", err)
	}

	// Should have seeded environments from migrations
	if len(envs) == 0 {
		t.Error("GetAllEnvironments() returned empty list, expected seeded data")
	}
}
