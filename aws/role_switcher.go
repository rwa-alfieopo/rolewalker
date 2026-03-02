package aws

import (
	"fmt"
	"strings"

	"rolewalkers/internal/db"
)

// RoleSwitcher handles switching between AWS roles
type RoleSwitcher struct {
	configManager *ConfigManager
	dbRepo        *db.ConfigRepository
}

// NewRoleSwitcher creates a new role switcher with a shared ConfigManager.
func NewRoleSwitcher(cm *ConfigManager, dbRepo *db.ConfigRepository) *RoleSwitcher {
	return &RoleSwitcher{
		configManager: cm,
		dbRepo:        dbRepo,
	}
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
			settings := ProfileSettings{Lines: rs.formatRoleSettings(role, account)}
			if err := writeDefaultSection(rs.configManager.configPath, settings); err != nil {
				return fmt.Errorf("failed to update AWS config: %w", err)
			}
		}
	} else {
		// Fall back to manual update
		settings := ProfileSettings{Lines: rs.formatRoleSettings(role, account)}
		if err := writeDefaultSection(rs.configManager.configPath, settings); err != nil {
			return fmt.Errorf("failed to update AWS config: %w", err)
		}
	}

	// Write unified active identity file
	if err := writeActiveIdentityFile(profileName); err != nil {
		return fmt.Errorf("failed to write active identity file: %w", err)
	}

	// Apply env vars and write env file using shared helper
	if err := applyProfileEnv(profileName, role.Region); err != nil {
		return fmt.Errorf("failed to apply environment: %w", err)
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
	fmt.Fprintf(&sb, "Profile: %s\n", role.ProfileName)
	fmt.Fprintf(&sb, "  Account: %s (%s)\n", account.AccountName, account.AccountID)
	fmt.Fprintf(&sb, "  Role: %s\n", role.RoleName)
	fmt.Fprintf(&sb, "  Region: %s\n", role.Region)
	
	if role.RoleARN.Valid && role.RoleARN.String != "" {
		fmt.Fprintf(&sb, "  ARN: %s\n", role.RoleARN.String)
	}
	
	if account.SSOStartURL.Valid && account.SSOStartURL.String != "" {
		fmt.Fprintf(&sb, "  SSO: %s\n", account.SSOStartURL.String)
	}
	
	return sb.String()
}
