package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProfileSettings holds the key-value pairs to write into the [default] section
// of ~/.aws/config. Both ProfileSwitcher and RoleSwitcher produce these.
type ProfileSettings struct {
	Lines []string // e.g. ["region = eu-west-2", "sso_account_id = 123456"]
}

// writeDefaultSection rewrites the [default] section in the AWS config file
// with the given settings. If no [default] section exists, one is prepended.
func writeDefaultSection(configPath string, settings ProfileSettings) error {
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inDefault := false
	defaultWritten := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if matches := configProfileRegex.FindStringSubmatch(trimmed); matches != nil {
			if inDefault {
				inDefault = false
			}
			if matches[1] == "default" {
				inDefault = true
				defaultWritten = true
				newLines = append(newLines, "[default]")
				newLines = append(newLines, settings.Lines...)
				continue
			}
		}

		if inDefault {
			if strings.Contains(trimmed, "=") || trimmed == "" {
				continue
			}
		}

		newLines = append(newLines, line)
	}

	if !defaultWritten {
		header := []string{"[default]"}
		header = append(header, settings.Lines...)
		header = append(header, "")
		newLines = append(header, newLines...)
	}

	return os.WriteFile(configPath, []byte(strings.Join(newLines, "\n")), 0600)
}

// applyProfileEnv sets the standard AWS environment variables for the current
// process and writes the env file for shell sourcing. Called by both
// ProfileSwitcher and RoleSwitcher after updating the config file.
func applyProfileEnv(profileName, region string) error {
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	os.Unsetenv("AWS_SESSION_TOKEN")

	os.Setenv("AWS_PROFILE", profileName)
	if region != "" {
		os.Setenv("AWS_DEFAULT_REGION", region)
		os.Setenv("AWS_REGION", region)
	}

	return writeEnvFile(profileName, region)
}

// writeActiveIdentityFile writes the active profile/role name to
// ~/.rolewalkers/active_identity. Replaces the old separate
// active_profile and active_role files.
func writeActiveIdentityFile(profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(rwDir, 0700); err != nil {
		return err
	}

	// Write unified file
	activeFile := filepath.Join(rwDir, "active_identity")
	if err := os.WriteFile(activeFile, []byte(profileName), 0600); err != nil {
		return err
	}

	// Also write legacy files for backward compatibility
	_ = os.WriteFile(filepath.Join(rwDir, "active_profile"), []byte(profileName), 0600)
	_ = os.WriteFile(filepath.Join(rwDir, "active_role"), []byte(profileName), 0600)

	return nil
}

// GetActiveProfile returns the currently active profile name.
// It reads from the unified active_identity file, falling back to
// the legacy active_profile file for backward compatibility.
func (cm *ConfigManager) GetActiveProfile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "default"
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")

	// Try unified file first
	data, err := os.ReadFile(filepath.Join(rwDir, "active_identity"))
	if err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}

	// Fall back to legacy file
	data, err = os.ReadFile(filepath.Join(rwDir, "active_profile"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}

	return "default"
}
