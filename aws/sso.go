package aws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// NewSSOManager creates a new SSO manager with a shared ConfigManager.
func NewSSOManager(cm *ConfigManager) (*SSOManager, error) {
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

	profile, err := FindProfileByName(profiles, profileName)
	if err != nil {
		return err
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

// LoginWithBrowser is an alias for Login (kept for interface compatibility).
// Deprecated: use Login directly.
func (sm *SSOManager) LoginWithBrowser(profileName string) error {
	return sm.Login(profileName)
}

// GetCachedToken retrieves cached SSO token for a start URL
func (sm *SSOManager) GetCachedToken(startURL string) (*SSOCache, error) {
	hash := sha256.Sum256([]byte(startURL))
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

	p, err := FindProfileByName(profiles, profileName)
	if err != nil || !p.IsSSO {
		return false
	}

	_, err = sm.GetCachedToken(p.SSOStartURL)
	return err == nil
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

// GetCredentialExpiry returns when credentials expire for a profile
func (sm *SSOManager) GetCredentialExpiry(profileName string) (*time.Time, error) {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return nil, err
	}

	p, err := FindProfileByName(profiles, profileName)
	if err != nil {
		return nil, err
	}
	if !p.IsSSO {
		return nil, fmt.Errorf("profile '%s' is not an SSO profile", profileName)
	}

	cache, err := sm.GetCachedToken(p.SSOStartURL)
	if err != nil {
		return nil, err
	}
	return &cache.ExpiresAt, nil
}

// ValidateProfile checks if a profile configuration is valid
func (sm *SSOManager) ValidateProfile(profileName string) error {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return err
	}

	p, err := FindProfileByName(profiles, profileName)
	if err != nil {
		return err
	}

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


