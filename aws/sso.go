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

// GetCachedToken retrieves cached SSO token for a profile.
// AWS CLI uses different cache keys depending on config style:
//   - sso_session profiles: SHA1(session_name)
//   - direct sso_start_url profiles: SHA1(start_url)
func (sm *SSOManager) GetCachedToken(startURL string) (*SSOCache, error) {
	return sm.findCachedToken(startURL)
}

// findCachedToken tries to find a valid SSO token in the cache.
// It checks the given key first, then scans all cache files as fallback.
func (sm *SSOManager) findCachedToken(cacheKey string) (*SSOCache, error) {
	// Try direct lookup with SHA1 (AWS CLI's algorithm)
	if cache, err := sm.readCacheFile(sha1Hex(cacheKey)); err == nil {
		return cache, nil
	}

	// Fallback: scan all cache files for a valid token matching this start URL
	// (handles cases where the cache key doesn't match our expectation)
	entries, err := os.ReadDir(sm.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("no cached token found")
	}

	for _, entry := range entries {
		if entry.IsDir() || !isJSONFile(entry.Name()) {
			continue
		}
		cache, err := sm.readCacheFile(trimJSONExt(entry.Name()))
		if err != nil {
			continue
		}
		// Match by start URL if present
		if cache.StartURL != "" && cache.StartURL == cacheKey {
			return cache, nil
		}
	}

	return nil, fmt.Errorf("no valid cached token found")
}

// readCacheFile reads and validates a single SSO cache file by its hash name.
func (sm *SSOManager) readCacheFile(hashName string) (*SSOCache, error) {
	cacheFile := filepath.Join(sm.cacheDir, hashName+".json")

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	var cache SSOCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	// Must have an access token to be a valid SSO session cache
	if cache.AccessToken == "" {
		return nil, fmt.Errorf("not an SSO token cache file")
	}

	if time.Now().After(cache.ExpiresAt) {
		return nil, fmt.Errorf("cached token expired")
	}

	return &cache, nil
}

// IsLoggedIn checks if SSO session is valid for a profile.
// Profiles using sso_session share a single token — if one is logged in, all are.
func (sm *SSOManager) IsLoggedIn(profileName string) bool {
	profiles, err := sm.configManager.GetProfiles()
	if err != nil {
		return false
	}

	p, err := FindProfileByName(profiles, profileName)
	if err != nil || !p.IsSSO {
		return false
	}

	// Try sso_session name first (AWS CLI caches by session name)
	if p.SSOSession != "" {
		_, err = sm.findCachedToken(p.SSOSession)
		return err == nil
	}

	// Fall back to start URL for direct sso_start_url profiles
	_, err = sm.findCachedToken(p.SSOStartURL)
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

	// Try sso_session name first
	cacheKey := p.SSOStartURL
	if p.SSOSession != "" {
		cacheKey = p.SSOSession
	}

	cache, err := sm.findCachedToken(cacheKey)
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

// --- Helpers ---

// sha1Hex returns the hex-encoded SHA1 hash of s.
func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func isJSONFile(name string) bool {
	return strings.HasSuffix(name, ".json")
}

func trimJSONExt(name string) string {
	return strings.TrimSuffix(name, ".json")
}