package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

const rwDirName = ".rolewalkers"

// RoleWalkersDir returns the path to ~/.rolewalkers, creating it if needed.
func RoleWalkersDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	dir := filepath.Join(homeDir, rwDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create %s directory: %w", rwDirName, err)
	}

	return dir, nil
}

// ReadRoleWalkersFile reads a file from ~/.rolewalkers/<name>.
// Returns the content or an error (including os.ErrNotExist).
func ReadRoleWalkersFile(name string) ([]byte, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(homeDir, rwDirName, name))
}

// WriteRoleWalkersFile writes data to ~/.rolewalkers/<name>, creating the
// directory if needed. Uses 0600 permissions.
func WriteRoleWalkersFile(name string, data []byte) error {
	dir, err := RoleWalkersDir()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0600)
}
