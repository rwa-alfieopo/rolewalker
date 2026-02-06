package aws

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Profile represents an AWS profile configuration
type Profile struct {
	Name         string `json:"name"`
	SSOStartURL  string `json:"ssoStartUrl,omitempty"`
	SSORegion    string `json:"ssoRegion,omitempty"`
	SSOAccountID string `json:"ssoAccountId,omitempty"`
	SSORoleName  string `json:"ssoRoleName,omitempty"`
	Region       string `json:"region,omitempty"`
	Output       string `json:"output,omitempty"`
	IsSSO        bool   `json:"isSso"`
	IsActive     bool   `json:"isActive"`
}

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
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
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
	profileRegex := regexp.MustCompile(`^\[(?:profile\s+)?(.+)\]$`)

	var currentProfile *Profile

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if matches := profileRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			if name == "default" {
				currentProfile = &Profile{Name: "default"}
			} else {
				currentProfile = &Profile{Name: name}
			}
			profiles[currentProfile.Name] = currentProfile
			continue
		}

		if currentProfile != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "sso_start_url":
				currentProfile.SSOStartURL = value
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

	return scanner.Err()
}
