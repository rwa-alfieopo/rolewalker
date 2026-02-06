package aws

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ProfileSwitcher handles switching between AWS profiles
type ProfileSwitcher struct {
	configManager *ConfigManager
}

// NewProfileSwitcher creates a new profile switcher
func NewProfileSwitcher() (*ProfileSwitcher, error) {
	cm, err := NewConfigManager()
	if err != nil {
		return nil, err
	}

	return &ProfileSwitcher{
		configManager: cm,
	}, nil
}

// SwitchProfile sets the active profile by updating default profile
func (ps *ProfileSwitcher) SwitchProfile(profileName string) error {
	profiles, err := ps.configManager.GetProfiles()
	if err != nil {
		return err
	}

	// Find the target profile
	var targetProfile *Profile
	for _, p := range profiles {
		if p.Name == profileName {
			targetProfile = &p
			break
		}
	}

	if targetProfile == nil {
		return fmt.Errorf("profile '%s' not found", profileName)
	}

	// Update the default profile in config
	if err := ps.updateDefaultProfile(targetProfile); err != nil {
		return err
	}

	// Also set environment variable hint file for shell integration
	if err := ps.writeActiveProfileFile(profileName); err != nil {
		return err
	}

	// Set persistent environment variable (Windows User level, or export file for Unix)
	if err := ps.setPersistentEnv(profileName, targetProfile.Region); err != nil {
		// Non-fatal - just warn
		fmt.Printf("⚠ Could not set persistent environment: %v\n", err)
	}

	return nil
}

// setPersistentEnv clears AWS_PROFILE from User environment on Windows
// so that AWS CLI uses the [default] profile from config file
func (ps *ProfileSwitcher) setPersistentEnv(profileName, region string) error {
	if runtime.GOOS == "windows" {
		// Clear AWS_PROFILE from User environment so AWS CLI uses [default] from config
		cmd := exec.Command("powershell", "-NoProfile", "-Command",
			`[Environment]::SetEnvironmentVariable("AWS_PROFILE", $null, "User")`)
		cmd.Run() // Non-fatal if this fails
	}
	return nil
}

// updateDefaultProfile updates the [default] section in AWS config
func (ps *ProfileSwitcher) updateDefaultProfile(profile *Profile) error {
	// Read existing config
	content, err := os.ReadFile(ps.configManager.configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inDefault := false
	defaultWritten := false
	profileRegex := regexp.MustCompile(`^\[(?:profile\s+)?(.+)\]$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if matches := profileRegex.FindStringSubmatch(trimmed); matches != nil {
			if inDefault {
				// We were in default section, now entering new section
				inDefault = false
			}
			if matches[1] == "default" {
				inDefault = true
				defaultWritten = true
				// Write new default section
				newLines = append(newLines, "[default]")
				newLines = append(newLines, ps.formatProfileSettings(profile)...)
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
		header = append(header, ps.formatProfileSettings(profile)...)
		header = append(header, "")
		newLines = append(header, newLines...)
	}

	// Write back
	return os.WriteFile(ps.configManager.configPath, []byte(strings.Join(newLines, "\n")), 0600)
}

// formatProfileSettings returns config lines for a profile
func (ps *ProfileSwitcher) formatProfileSettings(profile *Profile) []string {
	var lines []string

	if profile.Region != "" {
		lines = append(lines, fmt.Sprintf("region = %s", profile.Region))
	}
	if profile.Output != "" {
		lines = append(lines, fmt.Sprintf("output = %s", profile.Output))
	}
	if profile.IsSSO {
		lines = append(lines, fmt.Sprintf("sso_start_url = %s", profile.SSOStartURL))
		lines = append(lines, fmt.Sprintf("sso_region = %s", profile.SSORegion))
		lines = append(lines, fmt.Sprintf("sso_account_id = %s", profile.SSOAccountID))
		lines = append(lines, fmt.Sprintf("sso_role_name = %s", profile.SSORoleName))
	}

	return lines
}

// writeActiveProfileFile writes the active profile name to a file
func (ps *ProfileSwitcher) writeActiveProfileFile(profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(rwDir, 0700); err != nil {
		return err
	}

	activeFile := filepath.Join(rwDir, "active_profile")
	return os.WriteFile(activeFile, []byte(profileName), 0600)
}

// GetActiveProfile returns the currently active profile name
func (cm *ConfigManager) GetActiveProfile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "default"
	}

	activeFile := filepath.Join(homeDir, ".rolewalkers", "active_profile")
	data, err := os.ReadFile(activeFile)
	if err != nil {
		return "default"
	}

	return strings.TrimSpace(string(data))
}

// GetDefaultRegion returns the region from the default profile
func (ps *ProfileSwitcher) GetDefaultRegion() string {
	file, err := os.Open(ps.configManager.configPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inDefault := false
	profileRegex := regexp.MustCompile(`^\[(?:profile\s+)?(.+)\]$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if matches := profileRegex.FindStringSubmatch(line); matches != nil {
			inDefault = matches[1] == "default"
			continue
		}

		if inDefault && strings.HasPrefix(line, "region") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

// ExportEnvironment returns environment variables for the active profile
func (ps *ProfileSwitcher) ExportEnvironment(profileName string) (map[string]string, error) {
	profiles, err := ps.configManager.GetProfiles()
	if err != nil {
		return nil, err
	}

	var profile *Profile
	for _, p := range profiles {
		if p.Name == profileName {
			profile = &p
			break
		}
	}

	if profile == nil {
		return nil, fmt.Errorf("profile '%s' not found", profileName)
	}

	env := make(map[string]string)
	env["AWS_PROFILE"] = profileName

	if profile.Region != "" {
		env["AWS_DEFAULT_REGION"] = profile.Region
		env["AWS_REGION"] = profile.Region
	}

	return env, nil
}

// GenerateShellExport generates shell export commands
func (ps *ProfileSwitcher) GenerateShellExport(profileName string, shell string) (string, error) {
	env, err := ps.ExportEnvironment(profileName)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	switch shell {
	case "powershell", "pwsh":
		for k, v := range env {
			sb.WriteString(fmt.Sprintf("$env:%s = '%s'\n", k, v))
		}
	case "cmd":
		for k, v := range env {
			sb.WriteString(fmt.Sprintf("set %s=%s\n", k, v))
		}
	default: // bash, zsh, sh
		for k, v := range env {
			sb.WriteString(fmt.Sprintf("export %s='%s'\n", k, v))
		}
	}

	return sb.String(), nil
}

// ClearEnvironment returns commands to clear AWS environment variables
func (ps *ProfileSwitcher) ClearEnvironment(shell string) string {
	vars := []string{"AWS_PROFILE", "AWS_DEFAULT_REGION", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}

	var sb strings.Builder

	switch shell {
	case "powershell", "pwsh":
		for _, v := range vars {
			sb.WriteString(fmt.Sprintf("Remove-Item Env:%s -ErrorAction SilentlyContinue\n", v))
		}
	case "cmd":
		for _, v := range vars {
			sb.WriteString(fmt.Sprintf("set %s=\n", v))
		}
	default:
		for _, v := range vars {
			sb.WriteString(fmt.Sprintf("unset %s\n", v))
		}
	}

	return sb.String()
}
