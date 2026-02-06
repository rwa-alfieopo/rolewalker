package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the SQLite database connection
type DB struct {
	*sql.DB
}

// NewDB creates a new database connection
func NewDB() (*DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	dbDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	dbPath := filepath.Join(dbDir, "config.db")
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for concurrent access (web server + CLI)
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}
	// Set a busy timeout so concurrent access waits instead of failing immediately
	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Set connection pool limits (keep low for local SQLite)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{sqlDB}

	// Run migrations
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// migrate runs all database migrations
func (db *DB) migrate() error {
	// Create migrations table
	if err := db.createMigrationsTable(); err != nil {
		return err
	}

	// Run migrations in order
	migrations := []struct {
		version int
		name    string
		up      func(*DB) error
	}{
		{1, "create_environments", migrateV1CreateEnvironments},
		{2, "create_services", migrateV2CreateServices},
		{3, "create_port_mappings", migrateV3CreatePortMappings},
		{4, "create_scaling_presets", migrateV4CreateScalingPresets},
		{5, "create_api_endpoints", migrateV5CreateAPIEndpoints},
		{6, "create_cluster_mappings", migrateV6CreateClusterMappings},
		{7, "seed_default_data", migrateV7SeedDefaultData},
		{8, "create_aws_accounts", migrateV8CreateAWSAccounts},
		{9, "create_aws_roles", migrateV9CreateAWSRoles},
		{10, "create_user_sessions", migrateV10CreateUserSessions},
	}

	for _, m := range migrations {
		if err := db.runMigration(m.version, m.name, m.up); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", m.version, m.name, err)
		}
	}

	return nil
}

// createMigrationsTable creates the migrations tracking table
func (db *DB) createMigrationsTable() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// runMigration runs a single migration if not already applied
func (db *DB) runMigration(version int, name string, up func(*DB) error) error {
	// Check if already applied
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM migrations WHERE version = ?", version).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil // Already applied
	}

	// Run migration
	if err := up(db); err != nil {
		return err
	}

	// Record migration
	_, err = db.Exec("INSERT INTO migrations (version, name) VALUES (?, ?)", version, name)
	return err
}
