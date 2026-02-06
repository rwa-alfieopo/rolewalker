package aws

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"rolewalkers/internal/db"
)

// ConfigSync handles synchronization between ~/.aws/config and SQLite database
type ConfigSync struct {
	configPath string
	dbRepo     *db.ConfigRepository
}

// SyncResult holds the result of a config sync operation
type SyncResult struct {
	Imported  int
	Updated   int
	Skipped   int
	Removed   int
	Errors    []string
	IsFirstRun bool
}

// ConfigProfile represents a parsed profile from ~/.aws/config
type ConfigProfile struct {
	Name         string
	Region       string
	Output       string
	SSOStartURL  string
	SSORegion    string
	SSOAccountID string
	SSORoleName  string
	SSOSession   string
	RoleARN      string
	IsSSO        bool
}

// NewConfigSync creates a new config sync manager
func NewConfigSync(dbRepo *db.ConfigRepository) (*ConfigSync, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	return &ConfigSync{
		configPath: filepath.Join(homeDir, ".aws", "config"),
		dbRepo:     dbRepo,
	}, nil
}

// ParseAWSConfigFile reads and parses ~/.aws/config into ConfigProfile structs
func (cs *ConfigSync) ParseAWSConfigFile() ([]ConfigProfile, error) {
	file, err := os.Open(cs.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	profileRegex := regexp.MustCompile(`^\[(?:profile\s+)?(.+)\]$`)

	var profiles []ConfigProfile
	var current *ConfigProfile

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := profileRegex.FindStringSubmatch(line); matches != nil {
			// Save previous profile
			if current != nil && !strings.HasPrefix(current.Name, "sso-session") {
				profiles = append(profiles, *current)
			}

			name := matches[1]
			// Skip sso-session sections
			if strings.HasPrefix(name, "sso-session") {
				current = &ConfigProfile{Name: name}
				continue
			}

			current = &ConfigProfile{Name: name}
			continue
		}

		if current != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "sso_start_url":
				current.SSOStartURL = value
				current.IsSSO = true
			case "sso_region":
				current.SSORegion = value
			case "sso_account_id":
				current.SSOAccountID = value
				current.IsSSO = true
			case "sso_role_name":
				current.SSORoleName = value
			case "sso_session":
				current.SSOSession = value
				current.IsSSO = true
			case "region":
				current.Region = value
			case "output":
				current.Output = value
			case "role_arn":
				current.RoleARN = value
			}
		}
	}

	// Don't forget the last profile
	if current != nil && !strings.HasPrefix(current.Name, "sso-session") {
		profiles = append(profiles, *current)
	}

	return profiles, scanner.Err()
}

// HasExistingData checks if the database already has AWS accounts/roles
func (cs *ConfigSync) HasExistingData() bool {
	accounts, err := cs.dbRepo.GetAllAWSAccounts()
	if err != nil {
		return false
	}
	return len(accounts) > 0
}

// ConfigFileExists checks if ~/.aws/config exists
func (cs *ConfigSync) ConfigFileExists() bool {
	_, err := os.Stat(cs.configPath)
	return err == nil
}

// AnalyzeSync compares the config file with the database and returns what would change
func (cs *ConfigSync) AnalyzeSync() (*SyncResult, error) {
	profiles, err := cs.ParseAWSConfigFile()
	if err != nil {
		return nil, err
	}

	if len(profiles) == 0 {
		return &SyncResult{}, nil
	}

	result := &SyncResult{
		IsFirstRun: !cs.HasExistingData(),
	}

	for _, p := range profiles {
		if p.Name == "default" || p.SSOAccountID == "" {
			result.Skipped++
			continue
		}

		// Check if role already exists in DB
		existingRole, _ := cs.dbRepo.GetRoleByProfileName(p.Name)
		if existingRole != nil {
			// Check if it needs updating
			needsUpdate := false
			if existingRole.Region != p.Region && p.Region != "" {
				needsUpdate = true
			}
			if existingRole.RoleName != p.SSORoleName && p.SSORoleName != "" {
				needsUpdate = true
			}
			if needsUpdate {
				result.Updated++
			} else {
				result.Skipped++
			}
		} else {
			result.Imported++
		}
	}

	return result, nil
}

// SyncConfigToDB imports profiles from ~/.aws/config into the SQLite database
func (cs *ConfigSync) SyncConfigToDB() (*SyncResult, error) {
	profiles, err := cs.ParseAWSConfigFile()
	if err != nil {
		return nil, err
	}

	result := &SyncResult{
		IsFirstRun: !cs.HasExistingData(),
	}

	if len(profiles) == 0 {
		return result, nil
	}

	// Resolve sso_session references - find the sso-session block's start_url
	ssoSessionURLs := cs.extractSSOSessionURLs()

	for _, p := range profiles {
		if p.Name == "default" {
			result.Skipped++
			continue
		}

		// Resolve sso_session to sso_start_url if needed
		if p.SSOSession != "" && p.SSOStartURL == "" {
			if url, ok := ssoSessionURLs[p.SSOSession]; ok {
				p.SSOStartURL = url
			}
			if region, ok := ssoSessionRegions(cs.configPath)[p.SSOSession]; ok && p.SSORegion == "" {
				p.SSORegion = region
			}
		}

		if p.SSOAccountID == "" {
			result.Skipped++
			continue
		}

		// Get or create the AWS account
		account, err := cs.dbRepo.GetAWSAccount(p.SSOAccountID)
		if err != nil || account == nil {
			// Create the account
			accountName := cs.deriveAccountName(p.Name)
			ssoRegion := p.SSORegion
			if ssoRegion == "" {
				ssoRegion = "eu-west-2"
			}

			if err := cs.dbRepo.AddAWSAccount(p.SSOAccountID, accountName, p.SSOStartURL, ssoRegion, "Imported from AWS config"); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to create account: %v", p.Name, err))
				continue
			}

			account, err = cs.dbRepo.GetAWSAccount(p.SSOAccountID)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: account created but not retrievable: %v", p.Name, err))
				continue
			}
		}

		// Check if role already exists
		existingRole, _ := cs.dbRepo.GetRoleByProfileName(p.Name)
		if existingRole != nil {
			// Update if changed
			needsUpdate := false
			updates := make(map[string]interface{})

			if p.Region != "" && existingRole.Region != p.Region {
				updates["region"] = p.Region
				needsUpdate = true
			}
			if p.SSORoleName != "" && existingRole.RoleName != p.SSORoleName {
				updates["role_name"] = p.SSORoleName
				needsUpdate = true
			}

			if needsUpdate {
				if err := cs.dbRepo.UpdateAWSRole(existingRole.ID, updates); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to update role: %v", p.Name, err))
				} else {
					result.Updated++
				}
			} else {
				result.Skipped++
			}
			continue
		}

		// Create new role
		roleName := p.SSORoleName
		if roleName == "" {
			roleName = "Role"
		}
		region := p.Region
		if region == "" {
			region = "eu-west-2"
		}

		if err := cs.dbRepo.AddAWSRole(account.ID, roleName, p.RoleARN, p.Name, region, "Imported from AWS config"); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				result.Skipped++
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: failed to create role: %v", p.Name, err))
			}
			continue
		}

		result.Imported++
	}

	return result, nil
}

// GenerateAWSConfig generates ~/.aws/config content from the database
func (cs *ConfigSync) GenerateAWSConfig() (string, error) {
	accounts, err := cs.dbRepo.GetAllAWSAccounts()
	if err != nil {
		return "", fmt.Errorf("failed to get accounts: %w", err)
	}

	var sb strings.Builder

	// Collect unique SSO start URLs to generate [sso-session] blocks
	ssoSessions := make(map[string]string) // sessionName -> startURL
	ssoSessionRegions := make(map[string]string)
	for _, account := range accounts {
		if account.SSOStartURL.Valid && account.SSOStartURL.String != "" {
			// Derive a session name from the account name or URL
			sessionName := cs.deriveSSOSessionName(&account)
			ssoSessions[sessionName] = account.SSOStartURL.String
			if account.SSORegion.Valid && account.SSORegion.String != "" {
				ssoSessionRegions[sessionName] = account.SSORegion.String
			}
		}
	}

	// Write [sso-session] blocks first
	for sessionName, startURL := range ssoSessions {
		sb.WriteString(fmt.Sprintf("[sso-session %s]\n", sessionName))
		sb.WriteString(fmt.Sprintf("sso_start_url = %s\n", startURL))
		if region, ok := ssoSessionRegions[sessionName]; ok {
			sb.WriteString(fmt.Sprintf("sso_region = %s\n", region))
		}
		sb.WriteString("sso_registration_scopes = sso:account:access\n")
		sb.WriteString("\n")
	}

	// Find the active session to write as [default]
	_, activeRole, activeAccount, err := cs.dbRepo.GetActiveSession()
	if err == nil && activeRole != nil && activeAccount != nil {
		sb.WriteString("[default]\n")
		sb.WriteString(fmt.Sprintf("region = %s\n", activeRole.Region))
		sb.WriteString("output = json\n")
		if activeAccount.SSOStartURL.Valid && activeAccount.SSOStartURL.String != "" {
			sessionName := cs.deriveSSOSessionName(activeAccount)
			sb.WriteString(fmt.Sprintf("sso_session = %s\n", sessionName))
			sb.WriteString(fmt.Sprintf("sso_account_id = %s\n", activeAccount.AccountID))
			sb.WriteString(fmt.Sprintf("sso_role_name = %s\n", activeRole.RoleName))
		}
		sb.WriteString("\n")
	}

	// Write all roles as named profiles
	for _, account := range accounts {
		roles, err := cs.dbRepo.GetRolesByAccount(account.AccountID)
		if err != nil {
			continue
		}

		sessionName := ""
		if account.SSOStartURL.Valid && account.SSOStartURL.String != "" {
			sessionName = cs.deriveSSOSessionName(&account)
		}

		for _, role := range roles {
			sb.WriteString(fmt.Sprintf("[profile %s]\n", role.ProfileName))
			if sessionName != "" {
				sb.WriteString(fmt.Sprintf("sso_session = %s\n", sessionName))
				sb.WriteString(fmt.Sprintf("sso_account_id = %s\n", account.AccountID))
				sb.WriteString(fmt.Sprintf("sso_role_name = %s\n", role.RoleName))
			}
			if role.RoleARN.Valid && role.RoleARN.String != "" {
				sb.WriteString(fmt.Sprintf("role_arn = %s\n", role.RoleARN.String))
			}
			sb.WriteString(fmt.Sprintf("region = %s\n", role.Region))
			sb.WriteString("output = json\n")
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// deriveSSOSessionName generates a consistent sso-session name from an account
func (cs *ConfigSync) deriveSSOSessionName(account *db.AWSAccount) string {
	if !account.SSOStartURL.Valid {
		return "default"
	}
	// Use a consistent name based on the SSO URL domain
	url := account.SSOStartURL.String
	// Extract the subdomain: https://d-9c67711d98.awsapps.com/start -> d-9c67711d98
	if strings.Contains(url, "awsapps.com") {
		parts := strings.Split(url, "//")
		if len(parts) > 1 {
			host := strings.Split(parts[1], ".")[0]
			return host
		}
	}
	return "company-sso"
}

// WriteAWSConfig writes the generated config to ~/.aws/config
func (cs *ConfigSync) WriteAWSConfig() error {
	content, err := cs.GenerateAWSConfig()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(cs.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create .aws directory: %w", err)
	}

	return os.WriteFile(cs.configPath, []byte(content), 0600)
}

// BackupConfigFile creates a backup of the current ~/.aws/config
func (cs *ConfigSync) BackupConfigFile() (string, error) {
	backupPath := cs.configPath + ".bak"
	content, err := os.ReadFile(cs.configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// DeleteConfigFile removes ~/.aws/config (after backup)
func (cs *ConfigSync) DeleteConfigFile() error {
	return os.Remove(cs.configPath)
}

// deriveAccountName extracts a friendly name from the profile name
func (cs *ConfigSync) deriveAccountName(profileName string) string {
	name := strings.TrimPrefix(profileName, "zenith-")
	name = strings.TrimPrefix(name, "AdministratorAccess-")
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

// extractSSOSessionURLs parses the config file for [sso-session X] blocks
func (cs *ConfigSync) extractSSOSessionURLs() map[string]string {
	result := make(map[string]string)

	file, err := os.Open(cs.configPath)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	ssoSessionRegex := regexp.MustCompile(`^\[sso-session\s+(.+)\]$`)
	var currentSession string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := ssoSessionRegex.FindStringSubmatch(line); matches != nil {
			currentSession = matches[1]
			continue
		}

		if currentSession != "" && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key == "sso_start_url" {
				result[currentSession] = value
			}
		}

		if strings.HasPrefix(line, "[") {
			currentSession = ""
		}
	}

	return result
}

// ssoSessionRegions parses the config file for sso_region in [sso-session X] blocks
func ssoSessionRegions(configPath string) map[string]string {
	result := make(map[string]string)

	file, err := os.Open(configPath)
	if err != nil {
		return result
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	ssoSessionRegex := regexp.MustCompile(`^\[sso-session\s+(.+)\]$`)
	var currentSession string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if matches := ssoSessionRegex.FindStringSubmatch(line); matches != nil {
			currentSession = matches[1]
			continue
		}

		if currentSession != "" && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key == "sso_region" {
				result[currentSession] = value
			}
		}

		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[sso-session") {
			currentSession = ""
		}
	}

	return result
}

// GetConfigPath returns the path to the AWS config file
func (cs *ConfigSync) GetConfigPath() string {
	return cs.configPath
}
