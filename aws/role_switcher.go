package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rolewalkers/internal/db"
)

// RoleSwitcher handles switching between AWS roles
type RoleSwitcher struct {
	configManager *ConfigManager
	dbRepo        *db.ConfigRepository
}

// NewRoleSwitcher creates a new role switcher
func NewRoleSwitcher(dbRepo *db.ConfigRepository) (*RoleSwitcher, error) {
	cm, err := NewConfigManager()
	if err != nil {
		return nil, err
	}

	return &RoleSwitcher{
		configManager: cm,
		dbRepo:        dbRepo,
	}, nil
}

// SwitchRole switches to a specific role by profile name
func (rs *RoleSwitcher) SwitchRole(profileName string) error {
	// Get role from database
	role, err := rs.dbRepo.GetRoleByProfileName(profileName)
	if err != nil {
		return fmt.Errorf("failed to get role: %w", err)
	}

	// Get the AWS account for this role
	account, err := rs.getAccountForRole(role)
	if err != nil {
		return err
	}

	// Create session in database
	if err := rs.dbRepo.CreateUserSession(role.ID); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Generate AWS config from database (rw manages the config)
	configSync, err := NewConfigSync(rs.dbRepo)
	if err == nil {
		if err := configSync.WriteAWSConfig(); err != nil {
			// Non-fatal: fall back to manual update
			fmt.Printf("⚠ Could not regenerate config from DB: %v\n", err)
			if err := rs.updateDefaultProfileFromRole(role, account); err != nil {
				return fmt.Errorf("failed to update AWS config: %w", err)
			}
		}
	} else {
		// Fall back to manual update
		if err := rs.updateDefaultProfileFromRole(role, account); err != nil {
			return fmt.Errorf("failed to update AWS config: %w", err)
		}
	}

	// Write active profile file for shell integration
	if err := rs.writeActiveRoleFile(profileName); err != nil {
		return fmt.Errorf("failed to write active role file: %w", err)
	}

	// Auto-update AWS_PROFILE env var for current process and child processes
	os.Setenv("AWS_PROFILE", profileName)
	if role.Region != "" {
		os.Setenv("AWS_DEFAULT_REGION", role.Region)
		os.Setenv("AWS_REGION", role.Region)
	}

	// Write env file for shell sourcing
	if err := writeEnvFile(profileName, role.Region); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	return nil
}

// getAccountForRole finds the AWS account for a given role
func (rs *RoleSwitcher) getAccountForRole(role *db.AWSRole) (*db.AWSAccount, error) {
	account, err := rs.dbRepo.GetAWSAccount(fmt.Sprintf("%d", role.AccountID))
	if err != nil {
		accounts, err := rs.dbRepo.GetAllAWSAccounts()
		if err != nil {
			return nil, fmt.Errorf("failed to get accounts: %w", err)
		}

		for _, acc := range accounts {
			if acc.ID == role.AccountID {
				return &acc, nil
			}
		}

		return nil, fmt.Errorf("account not found for role")
	}
	return account, nil
}

// updateDefaultProfileFromRole updates the [default] section in AWS config
func (rs *RoleSwitcher) updateDefaultProfileFromRole(role *db.AWSRole, account *db.AWSAccount) error {
	// Read existing config
	content, err := os.ReadFile(rs.configManager.configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inDefault := false
	defaultWritten := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering a profile section
		if strings.HasPrefix(trimmed, "[") {
			if inDefault {
				inDefault = false
			}
			if trimmed == "[default]" {
				inDefault = true
				defaultWritten = true
				// Write new default section
				newLines = append(newLines, "[default]")
				newLines = append(newLines, rs.formatRoleSettings(role, account)...)
				continue
			}
		}

		if inDefault {
			// Skip old default settings
			if strings.Contains(trimmed, "=") || trimmed == "" {
				continue
			}
		}

		newLines = append(newLines, line)
	}

	// If no default section existed, add it at the beginning
	if !defaultWritten {
		header := []string{"[default]"}
		header = append(header, rs.formatRoleSettings(role, account)...)
		header = append(header, "")
		newLines = append(header, newLines...)
	}

	// Write back
	return os.WriteFile(rs.configManager.configPath, []byte(strings.Join(newLines, "\n")), 0600)
}

// formatRoleSettings returns config lines for a role
func (rs *RoleSwitcher) formatRoleSettings(role *db.AWSRole, account *db.AWSAccount) []string {
	var lines []string

	lines = append(lines, fmt.Sprintf("region = %s", role.Region))
	lines = append(lines, "output = json")

	// If SSO is configured for this account
	if account.SSOStartURL.Valid && account.SSOStartURL.String != "" {
		lines = append(lines, fmt.Sprintf("sso_start_url = %s", account.SSOStartURL.String))
		
		if account.SSORegion.Valid && account.SSORegion.String != "" {
			lines = append(lines, fmt.Sprintf("sso_region = %s", account.SSORegion.String))
		}
		
		lines = append(lines, fmt.Sprintf("sso_account_id = %s", account.AccountID))
		lines = append(lines, fmt.Sprintf("sso_role_name = %s", role.RoleName))
	} else if role.RoleARN.Valid && role.RoleARN.String != "" {
		// Use role ARN if available
		lines = append(lines, fmt.Sprintf("role_arn = %s", role.RoleARN.String))
	}

	return lines
}

// writeActiveRoleFile writes the active role profile name to a file
func (rs *RoleSwitcher) writeActiveRoleFile(profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(rwDir, 0700); err != nil {
		return err
	}

	activeFile := filepath.Join(rwDir, "active_role")
	return os.WriteFile(activeFile, []byte(profileName), 0600)
}

// GetActiveRole returns the currently active role
func (rs *RoleSwitcher) GetActiveRole() (*db.UserSession, *db.AWSRole, *db.AWSAccount, error) {
	return rs.dbRepo.GetActiveSession()
}

// ListRolesByAccount lists all roles for a given account
func (rs *RoleSwitcher) ListRolesByAccount(accountID string) ([]db.AWSRole, error) {
	return rs.dbRepo.GetRolesByAccount(accountID)
}

// ListAllAccounts lists all AWS accounts
func (rs *RoleSwitcher) ListAllAccounts() ([]db.AWSAccount, error) {
	return rs.dbRepo.GetAllAWSAccounts()
}

// FormatRoleInfo returns a formatted string with role details
func FormatRoleInfo(role db.AWSRole, account db.AWSAccount) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Profile: %s\n", role.ProfileName))
	sb.WriteString(fmt.Sprintf("  Account: %s (%s)\n", account.AccountName, account.AccountID))
	sb.WriteString(fmt.Sprintf("  Role: %s\n", role.RoleName))
	sb.WriteString(fmt.Sprintf("  Region: %s\n", role.Region))
	
	if role.RoleARN.Valid && role.RoleARN.String != "" {
		sb.WriteString(fmt.Sprintf("  ARN: %s\n", role.RoleARN.String))
	}
	
	if account.SSOStartURL.Valid && account.SSOStartURL.String != "" {
		sb.WriteString(fmt.Sprintf("  SSO: %s\n", account.SSOStartURL.String))
	}
	
	return sb.String()
}
