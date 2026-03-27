package aws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"rolewalkers/internal/awscli"
	"rolewalkers/internal/config"
	"rolewalkers/internal/db"
)

// SetupManager handles automatic discovery and configuration.
type SetupManager struct {
	dbRepo *db.ConfigRepository
	region string
}

// NewSetupManager creates a new SetupManager.
func NewSetupManager(dbRepo *db.ConfigRepository) *SetupManager {
	cfg := config.Get()
	return &SetupManager{dbRepo: dbRepo, region: cfg.Region}
}

// ssoAccountInfo represents an account from aws sso list-accounts.
type ssoAccountInfo struct {
	AccountID   string `json:"accountId"`
	AccountName string `json:"accountName"`
	EmailAddr   string `json:"emailAddress"`
}

// ssoRoleInfo represents a role from aws sso list-account-roles.
type ssoRoleInfo struct {
	RoleName  string `json:"roleName"`
	AccountID string `json:"accountId"`
}

// SetupResult holds the results of the setup process.
type SetupResult struct {
	Accounts    int
	Roles       int
	Clusters    int
	Profiles    int
	Errors      []string
}

// LoginAndDiscover performs the full setup flow:
// 1. Write a temporary SSO config
// 2. Login via browser
// 3. Discover accounts and roles
// 4. Discover EKS clusters
// 5. Generate AWS config and kubeconfig
func (sm *SetupManager) LoginAndDiscover(startURL, ssoRegion string) (*SetupResult, error) {
	cfg := config.Get()
	result := &SetupResult{}

	// Derive a session name from the start URL
	sessionName := deriveSessionName(startURL)

	// Step 1: Write a minimal temporary AWS config with sso-session for login
	fmt.Println("Setting up SSO configuration...")
	if err := sm.writeTempSSOConfig(sessionName, startURL, ssoRegion); err != nil {
		return nil, fmt.Errorf("failed to write SSO config: %w", err)
	}

	// Step 2: Login via browser
	fmt.Println("\nOpening browser for SSO authentication...")
	fmt.Println("Please complete the login in your browser.")
	if err := sm.ssoLogin(sessionName); err != nil {
		return nil, fmt.Errorf("SSO login failed: %w", err)
	}
	fmt.Println("✓ SSO login successful")

	// Step 3: Get the access token from cache
	fmt.Println("\nRetrieving access token...")
	cm, _ := NewConfigManager()
	ssoMgr, _ := NewSSOManager(cm)
	token, err := ssoMgr.findCachedToken(sessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSO token after login: %w", err)
	}

	// Step 4: Discover accounts
	fmt.Println("\nDiscovering AWS accounts...")
	accounts, err := sm.listAccounts(token.AccessToken, ssoRegion)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	fmt.Printf("  Found %d account(s)\n", len(accounts))
	result.Accounts = len(accounts)

	// Step 5: Discover roles per account and save to DB
	fmt.Println("\nDiscovering roles and setting up profiles...")
	var allProfiles []Profile
	for _, acc := range accounts {
		roles, err := sm.listAccountRoles(token.AccessToken, acc.AccountID, ssoRegion)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Account %s: failed to list roles: %v", acc.AccountID, err))
			continue
		}

		// Save account to DB
		if sm.dbRepo != nil {
			_ = sm.dbRepo.AddAWSAccount(acc.AccountID, acc.AccountName, startURL, ssoRegion, "Auto-discovered via rw setup")
		}

		for _, role := range roles {
			profileName := sm.buildProfileName(acc.AccountName, role.RoleName)

			// Save role to DB
			if sm.dbRepo != nil {
				account, _ := sm.dbRepo.GetAWSAccount(acc.AccountID)
				if account != nil {
					_ = sm.dbRepo.AddAWSRole(account.ID, role.RoleName, "", profileName, cfg.Region, "Auto-discovered via rw setup")
				}
			}

			allProfiles = append(allProfiles, Profile{
				Name:         profileName,
				SSOSession:   sessionName,
				SSOAccountID: acc.AccountID,
				SSORoleName:  role.RoleName,
				Region:       cfg.Region,
				IsSSO:        true,
			})
			result.Roles++

			fmt.Printf("  ✓ %s → %s (%s)\n", acc.AccountName, role.RoleName, profileName)
		}
	}
	result.Profiles = len(allProfiles)

	// Step 6: Generate ~/.aws/config
	fmt.Println("\nGenerating AWS config file...")
	if err := sm.writeAWSConfig(sessionName, startURL, ssoRegion, allProfiles); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to write AWS config: %v", err))
	} else {
		fmt.Println("  ✓ ~/.aws/config written")
	}

	// Step 7: Discover EKS clusters per account
	fmt.Println("\nDiscovering EKS clusters...")
	for _, p := range allProfiles {
		clusters, err := sm.listEKSClusters(p.Name)
		if err != nil {
			// Not all accounts have EKS — this is expected
			continue
		}

		for _, cluster := range clusters {
			fmt.Printf("  ✓ %s → %s\n", p.Name, cluster)
			result.Clusters++

			// Update kubeconfig
			if err := sm.updateKubeconfig(cluster, p.Name); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to update kubeconfig for %s: %v", cluster, err))
			}

			// Map cluster to environment and save to DB
			envName := sm.extractEnvFromCluster(cluster)
			if envName != "" && sm.dbRepo != nil {
				sm.upsertEnvironment(envName, p.Name, cluster)
			}
		}
	}

	return result, nil
}

// ssoLogin runs `aws sso login` with the temp profile.
func (sm *SetupManager) ssoLogin(sessionName string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", fmt.Sprintf("aws sso login --profile rw-setup"))
	} else {
		cmd = exec.Command("aws", "sso", "login", "--profile", "rw-setup")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Run()
}

// listAccounts calls aws sso list-accounts using the access token.
func (sm *SetupManager) listAccounts(accessToken, ssoRegion string) ([]ssoAccountInfo, error) {
	cmd := awscli.CreateCommand("sso", "list-accounts",
		"--access-token", accessToken,
		"--region", ssoRegion,
		"--output", "json",
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	var resp struct {
		AccountList []ssoAccountInfo `json:"accountList"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse accounts: %w", err)
	}

	return resp.AccountList, nil
}

// listAccountRoles calls aws sso list-account-roles for a specific account.
func (sm *SetupManager) listAccountRoles(accessToken, accountID, ssoRegion string) ([]ssoRoleInfo, error) {
	cmd := awscli.CreateCommand("sso", "list-account-roles",
		"--access-token", accessToken,
		"--account-id", accountID,
		"--region", ssoRegion,
		"--output", "json",
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	var resp struct {
		RoleList []ssoRoleInfo `json:"roleList"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse roles: %w", err)
	}

	return resp.RoleList, nil
}

// listEKSClusters calls aws eks list-clusters using a profile.
func (sm *SetupManager) listEKSClusters(profileName string) ([]string, error) {
	cmd := awscli.CreateCommand("eks", "list-clusters",
		"--profile", profileName,
		"--region", sm.region,
		"--output", "json",
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	var resp struct {
		Clusters []string `json:"clusters"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse clusters: %w", err)
	}

	return resp.Clusters, nil
}

// updateKubeconfig runs aws eks update-kubeconfig for a cluster.
func (sm *SetupManager) updateKubeconfig(clusterName, profileName string) error {
	cmd := awscli.CreateCommand("eks", "update-kubeconfig",
		"--name", clusterName,
		"--region", sm.region,
		"--profile", profileName,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

// writeTempSSOConfig writes a minimal ~/.aws/config with an sso-session
// and a temporary "rw-setup" profile for the initial login.
// It backs up the existing config first.
func (sm *SetupManager) writeTempSSOConfig(sessionName, startURL, ssoRegion string) error {
	cm, err := NewConfigManager()
	if err != nil {
		return err
	}

	// Back up existing config if it exists
	existing, _ := os.ReadFile(cm.configPath)
	if len(existing) > 0 {
		backupPath := cm.configPath + ".bak"
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
		fmt.Printf("  Backed up existing config to: %s\n", backupPath)
	}

	// Write a clean minimal config for SSO login only
	content := fmt.Sprintf(`[sso-session %s]
sso_start_url = %s
sso_region = %s
sso_registration_scopes = sso:account:access

[profile rw-setup]
sso_session = %s
sso_account_id = 000000000000
sso_role_name = placeholder
region = %s
`, sessionName, startURL, ssoRegion, sessionName, sm.region)

	return os.WriteFile(cm.configPath, []byte(content), 0600)
}

// writeAWSConfig generates the full ~/.aws/config from discovered profiles.
func (sm *SetupManager) writeAWSConfig(sessionName, startURL, ssoRegion string, profiles []Profile) error {
	cm, err := NewConfigManager()
	if err != nil {
		return err
	}

	var sb strings.Builder

	// Write sso-session block
	fmt.Fprintf(&sb, "[sso-session %s]\n", sessionName)
	fmt.Fprintf(&sb, "sso_start_url = %s\n", startURL)
	fmt.Fprintf(&sb, "sso_region = %s\n", ssoRegion)
	fmt.Fprintf(&sb, "sso_registration_scopes = sso:account:access\n\n")

	// Write default profile (first profile)
	if len(profiles) > 0 {
		p := profiles[0]
		sb.WriteString("[default]\n")
		fmt.Fprintf(&sb, "region = %s\n", p.Region)
		fmt.Fprintf(&sb, "output = json\n\n")
	}

	// Write each profile
	for _, p := range profiles {
		fmt.Fprintf(&sb, "[profile %s]\n", p.Name)
		fmt.Fprintf(&sb, "sso_session = %s\n", sessionName)
		fmt.Fprintf(&sb, "sso_account_id = %s\n", p.SSOAccountID)
		fmt.Fprintf(&sb, "sso_role_name = %s\n", p.SSORoleName)
		fmt.Fprintf(&sb, "region = %s\n", p.Region)
		fmt.Fprintf(&sb, "output = json\n\n")
	}

	return os.WriteFile(cm.configPath, []byte(sb.String()), 0600)
}

// buildProfileName creates a profile name from account name and role.
func (sm *SetupManager) buildProfileName(accountName, roleName string) string {
	// Normalize: "Zenith Dev" → "zenith-dev"
	name := strings.ToLower(accountName)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// If role is not the default admin role, append it
	roleLower := strings.ToLower(roleName)
	if roleLower != "administratoraccess" && roleLower != "admin" {
		name += "-" + strings.ToLower(roleName)
	}

	return name
}

// extractEnvFromCluster extracts the environment name from a cluster name.
// e.g. "dev-zenith-eks-cluster" → "dev"
func (sm *SetupManager) extractEnvFromCluster(clusterName string) string {
	cfg := config.Get()
	suffix := fmt.Sprintf("-%s-eks-cluster", cfg.Project)
	if strings.HasSuffix(clusterName, suffix) {
		return strings.TrimSuffix(clusterName, suffix)
	}
	// Try generic pattern: <env>-<anything>-eks-cluster
	if strings.HasSuffix(clusterName, "-eks-cluster") {
		parts := strings.SplitN(clusterName, "-", 2)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

// upsertEnvironment creates or updates an environment in the DB.
func (sm *SetupManager) upsertEnvironment(envName, profileName, clusterName string) {
	if sm.dbRepo == nil {
		return
	}

	// Check if environment already exists
	existing, err := sm.dbRepo.GetEnvironment(envName)
	if err == nil && existing != nil {
		// Already exists — skip
		return
	}

	displayName := strings.ToUpper(envName[:1]) + envName[1:]
	cfg := config.Get()

	// Insert new environment
	sm.dbRepo.AddEnvironment(envName, displayName, cfg.Region, profileName, clusterName)
}

// deriveSessionName extracts a session name from the SSO start URL.
// e.g. "https://d-9c67711d98.awsapps.com/start/#" → "d-9c67711d98"
func deriveSessionName(startURL string) string {
	// Extract the subdomain from the URL
	url := strings.TrimPrefix(startURL, "https://")
	url = strings.TrimPrefix(url, "http://")
	parts := strings.SplitN(url, ".", 2)
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "rw-sso"
}
