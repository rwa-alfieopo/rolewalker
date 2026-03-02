package aws

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ProfileSwitcher handles switching between AWS profiles
type ProfileSwitcher struct {
	configManager *ConfigManager
}

// NewProfileSwitcher creates a new profile switcher with a shared ConfigManager.
func NewProfileSwitcher(cm *ConfigManager) *ProfileSwitcher {
	return &ProfileSwitcher{
		configManager: cm,
	}
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

	// Update the [default] section in config using shared helper
	settings := ProfileSettings{Lines: ps.formatProfileSettings(targetProfile)}
	if err := writeDefaultSection(ps.configManager.configPath, settings); err != nil {
		return err
	}

	// Write unified active identity file
	if err := writeActiveIdentityFile(profileName); err != nil {
		return err
	}

	// Set persistent environment variable (Windows User level, or export file for Unix)
	if err := ps.setPersistentEnv(profileName, targetProfile.Region); err != nil {
		// Non-fatal - just warn
		fmt.Printf("⚠ Could not set persistent environment: %v\n", err)
	}

	// Apply env vars and write env file using shared helper
	if err := applyProfileEnv(profileName, targetProfile.Region); err != nil {
		return fmt.Errorf("failed to apply environment: %w", err)
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
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clear AWS_PROFILE from user environment: %w", err)
		}
	}
	return nil
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
		if profile.SSOSession != "" {
			lines = append(lines, fmt.Sprintf("sso_session = %s", profile.SSOSession))
		} else if profile.SSOStartURL != "" {
			lines = append(lines, fmt.Sprintf("sso_start_url = %s", profile.SSOStartURL))
			if profile.SSORegion != "" {
				lines = append(lines, fmt.Sprintf("sso_region = %s", profile.SSORegion))
			}
		}
		lines = append(lines, fmt.Sprintf("sso_account_id = %s", profile.SSOAccountID))
		lines = append(lines, fmt.Sprintf("sso_role_name = %s", profile.SSORoleName))
	}

	return lines
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

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if matches := configProfileRegex.FindStringSubmatch(line); matches != nil {
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
			fmt.Fprintf(&sb, "$env:%s = '%s'\n", k, v)
		}
	case "cmd":
		for k, v := range env {
			fmt.Fprintf(&sb, "set %s=%s\n", k, v)
		}
	default: // bash, zsh, sh
		for k, v := range env {
			fmt.Fprintf(&sb, "export %s='%s'\n", k, v)
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
			fmt.Fprintf(&sb, "Remove-Item Env:%s -ErrorAction SilentlyContinue\n", v)
		}
	case "cmd":
		for _, v := range vars {
			fmt.Fprintf(&sb, "set %s=\n", v)
		}
	default:
		for _, v := range vars {
			fmt.Fprintf(&sb, "unset %s\n", v)
		}
	}

	return sb.String()
}
