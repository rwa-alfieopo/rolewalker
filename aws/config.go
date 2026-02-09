package aws

import (
	"bufio"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Profile represents an AWS profile configuration
type Profile struct {
	Name         string `json:"name"`
	SSOSession   string `json:"ssoSession,omitempty"`
	SSOStartURL  string `json:"ssoStartUrl,omitempty"`
	SSORegion    string `json:"ssoRegion,omitempty"`
	SSOAccountID string `json:"ssoAccountId,omitempty"`
	SSORoleName  string `json:"ssoRoleName,omitempty"`
	Region       string `json:"region,omitempty"`
	Output       string `json:"output,omitempty"`
	IsSSO        bool   `json:"isSso"`
	IsActive     bool   `json:"isActive"`
}

// ssoSessionConfig holds settings from an [sso-session ...] block
type ssoSessionConfig struct {
	StartURL string
	Region   string
}

// Package-level compiled regexes for config parsing (avoids recompilation per call)
var (
	configProfileRegex    = regexp.MustCompile(`^\[(?:profile\s+)?(.+)\]$`)
	configSSOSessionRegex = regexp.MustCompile(`^\[sso-session\s+(.+)\]$`)
)

// ConfigManager handles AWS config file operations
type ConfigManager struct {
	configPath      string
	credentialsPath string
}

// NewConfigManager creates a new config manager
func NewConfigManager() (*ConfigManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	awsDir := filepath.Join(homeDir, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create .aws directory: %w", err)
	}

	return &ConfigManager{
		configPath:      filepath.Join(awsDir, "config"),
		credentialsPath: filepath.Join(awsDir, "credentials"),
	}, nil
}

// GetProfiles returns all configured AWS profiles
func (cm *ConfigManager) GetProfiles() ([]Profile, error) {
	profiles := make(map[string]*Profile)

	// Parse config file
	if err := cm.parseConfigFile(profiles); err != nil {
		return nil, err
	}

	// Get active profile
	activeProfile := cm.GetActiveProfile()

	// Convert map to slice
	result := make([]Profile, 0, len(profiles))
	for _, p := range profiles {
		p.IsActive = p.Name == activeProfile
		result = append(result, *p)
	}

	// Sort by name
	slices.SortFunc(result, func(a, b Profile) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result, nil
}

func (cm *ConfigManager) parseConfigFile(profiles map[string]*Profile) error {
	file, err := os.Open(cm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	ssoSessions := make(map[string]*ssoSessionConfig)

	var currentProfile *Profile
	var currentSession *ssoSessionConfig

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for [sso-session <name>] sections
		if matches := configSSOSessionRegex.FindStringSubmatch(line); matches != nil {
			currentProfile = nil
			currentSession = &ssoSessionConfig{}
			ssoSessions[matches[1]] = currentSession
			continue
		}

		if matches := configProfileRegex.FindStringSubmatch(line); matches != nil {
			currentSession = nil
			name := matches[1]
			if name == "default" {
				currentProfile = &Profile{Name: "default"}
			} else {
				currentProfile = &Profile{Name: name}
			}
			profiles[currentProfile.Name] = currentProfile
			continue
		}

		if currentSession != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "sso_start_url":
				currentSession.StartURL = value
			case "sso_region":
				currentSession.Region = value
			}
		}

		if currentProfile != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "sso_start_url":
				currentProfile.SSOStartURL = value
				currentProfile.IsSSO = true
			case "sso_session":
				currentProfile.SSOSession = value
				currentProfile.IsSSO = true
			case "sso_region":
				currentProfile.SSORegion = value
			case "sso_account_id":
				currentProfile.SSOAccountID = value
			case "sso_role_name":
				currentProfile.SSORoleName = value
			case "region":
				currentProfile.Region = value
			case "output":
				currentProfile.Output = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Resolve sso_session references: inherit start_url and region from the session block
	for _, p := range profiles {
		if p.SSOSession != "" {
			if sess, ok := ssoSessions[p.SSOSession]; ok {
				if p.SSOStartURL == "" {
					p.SSOStartURL = sess.StartURL
				}
				if p.SSORegion == "" {
					p.SSORegion = sess.Region
				}
			}
		}
	}

	return nil
}
