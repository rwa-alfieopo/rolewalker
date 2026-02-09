package aws

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SSOCache represents cached SSO credentials
type SSOCache struct {
	StartURL    string    `json:"startUrl"`
	Region      string    `json:"region"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// SSOCredentials represents temporary AWS credentials from SSO
type SSOCredentials struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey string    `json:"secretAccessKey"`
	SessionToken    string    `json:"sessionToken"`
	Expiration      time.Time `json:"expiration"`
}

// SSOManager handles AWS SSO operations
type SSOManager struct {
	configManager *ConfigManager
	cacheDir      string
}

// NewSSOManager creates a new SSO manager
func NewSSOManager() (*SSOManager, error) {
	cm, err := NewConfigManager()
	if err != nil {
		return nil, err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	return &SSOManager{
		configManager: cm,
		cacheDir:      filepath.Join(homeDir, ".aws", "sso", "cache"),
	}, nil
}

// Login initiates SSO login for a profile
func (sm *SSOManager) Login(profileName string) error {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return err
	}

	var profile *Profile
	for _, p := range profiles {
		if p.Name == profileName {
			profile = &p
			break
		}
	}

	if profile == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	if !profile.IsSSO {
		return fmt.Errorf("profile '%s' is not an SSO profile", profileName)
	}

	// Use AWS CLI for SSO login
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create command with proper OS-compatible execution
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// On Windows, cmd /C expects a single command string
		cmd = exec.CommandContext(ctx, "cmd", "/C", fmt.Sprintf("aws sso login --profile %s", profileName))
	} else {
		// On Unix-like systems (Linux, macOS), execute directly
		cmd = exec.CommandContext(ctx, "aws", "sso", "login", "--profile", profileName)
	}

	// Connect standard streams for interactive authentication
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set environment to ensure proper terminal handling
	cmd.Env = os.Environ()

	return cmd.Run()
}

// LoginWithBrowser opens browser for SSO login and returns when complete
func (sm *SSOManager) LoginWithBrowser(profileName string) error {
	return sm.Login(profileName)
}

// GetCachedToken retrieves cached SSO token for a start URL
func (sm *SSOManager) GetCachedToken(startURL string) (*SSOCache, error) {
	hash := sha1.Sum([]byte(startURL))
	cacheFile := filepath.Join(sm.cacheDir, hex.EncodeToString(hash[:])+".json")

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("no cached token found: %w", err)
	}

	var cache SSOCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}

	if time.Now().After(cache.ExpiresAt) {
		return nil, fmt.Errorf("cached token expired")
	}

	return &cache, nil
}

// IsLoggedIn checks if SSO session is valid for a profile
func (sm *SSOManager) IsLoggedIn(profileName string) bool {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return false
	}

	for _, p := range profiles {
		if p.Name == profileName && p.IsSSO {
			_, err := sm.GetCachedToken(p.SSOStartURL)
			return err == nil
		}
	}

	return false
}

// Logout clears SSO session
func (sm *SSOManager) Logout(profileName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// aws sso logout does not accept --profile; it clears all cached SSO tokens.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", "aws sso logout")
	} else {
		cmd = exec.CommandContext(ctx, "aws", "sso", "logout")
	}

	return cmd.Run()
}

// OpenBrowser opens the default browser with the given URL
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

// GetSSOProfiles returns only SSO-enabled profiles
func (sm *SSOManager) GetSSOProfiles() ([]Profile, error) {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return nil, err
	}

	ssoProfiles := make([]Profile, 0)
	for _, p := range profiles {
		if p.IsSSO {
			ssoProfiles = append(ssoProfiles, p)
		}
	}

	return ssoProfiles, nil
}

// GetStartURLs returns unique SSO start URLs
func (sm *SSOManager) GetStartURLs() ([]string, error) {
	profiles, err := sm.GetSSOProfiles()
	if err != nil {
		return nil, err
	}

	urlSet := make(map[string]bool)
	for _, p := range profiles {
		if p.SSOStartURL != "" {
			urlSet[p.SSOStartURL] = true
		}
	}

	urls := make([]string, 0, len(urlSet))
	for url := range urlSet {
		urls = append(urls, url)
	}

	return urls, nil
}

// RefreshCredentials refreshes credentials for a profile
func (sm *SSOManager) RefreshCredentials(profileName string) error {
	// Force re-login to refresh
	return sm.Login(profileName)
}

// GetCredentialExpiry returns when credentials expire for a profile
func (sm *SSOManager) GetCredentialExpiry(profileName string) (*time.Time, error) {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return nil, err
	}

	for _, p := range profiles {
		if p.Name == profileName && p.IsSSO {
			cache, err := sm.GetCachedToken(p.SSOStartURL)
			if err != nil {
				return nil, err
			}
			return &cache.ExpiresAt, nil
		}
	}

	return nil, fmt.Errorf("profile not found or not SSO")
}

// ValidateProfile checks if a profile configuration is valid
func (sm *SSOManager) ValidateProfile(profileName string) error {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return err
	}

	for _, p := range profiles {
		if p.Name == profileName {
			if p.IsSSO {
				if p.SSOStartURL == "" && p.SSOSession == "" {
					return fmt.Errorf("missing sso_start_url or sso_session")
				}
				if p.SSORegion == "" && p.SSOSession == "" {
					return fmt.Errorf("missing sso_region")
				}
				if p.SSOAccountID == "" {
					return fmt.Errorf("missing sso_account_id")
				}
				if p.SSORoleName == "" {
					return fmt.Errorf("missing sso_role_name")
				}
			}
			return nil
		}
	}

	return fmt.Errorf("profile '%s' not found", profileName)
}

// GetProfilesByStartURL groups profiles by their SSO start URL
func (sm *SSOManager) GetProfilesByStartURL() (map[string][]Profile, error) {
	profiles, err := sm.GetSSOProfiles()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]Profile)
	for _, p := range profiles {
		result[p.SSOStartURL] = append(result[p.SSOStartURL], p)
	}

	return result, nil
}

// GetAccountsForStartURL returns unique account IDs for a start URL
func (sm *SSOManager) GetAccountsForStartURL(startURL string) ([]string, error) {
	profiles, err := sm.GetSSOProfiles()
	if err != nil {
		return nil, err
	}

	accountSet := make(map[string]bool)
	for _, p := range profiles {
		if p.SSOStartURL == startURL && p.SSOAccountID != "" {
			accountSet[p.SSOAccountID] = true
		}
	}

	accounts := make([]string, 0, len(accountSet))
	for acc := range accountSet {
		accounts = append(accounts, acc)
	}

	return accounts, nil
}

// FormatProfileInfo returns a formatted string with profile details
func FormatProfileInfo(p Profile) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Profile: %s\n", p.Name)
	if p.Region != "" {
		fmt.Fprintf(&sb, "  Region: %s\n", p.Region)
	}
	if p.IsSSO {
		fmt.Fprintf(&sb, "  SSO Start URL: %s\n", p.SSOStartURL)
		fmt.Fprintf(&sb, "  SSO Region: %s\n", p.SSORegion)
		fmt.Fprintf(&sb, "  Account ID: %s\n", p.SSOAccountID)
		fmt.Fprintf(&sb, "  Role: %s\n", p.SSORoleName)
	}
	return sb.String()
}
