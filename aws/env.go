package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// safeShellValue matches only safe characters for shell variable values
var safeShellValue = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

// writeEnvFile writes AWS environment variables to ~/.rolewalkers/env
// so the shell can source it to pick up AWS_PROFILE automatically.
func writeEnvFile(profileName, region string) error {
	// Validate inputs to prevent shell injection
	if !safeShellValue.MatchString(profileName) {
		return fmt.Errorf("invalid profile name for shell export: %s", profileName)
	}
	if region != "" && !safeShellValue.MatchString(region) {
		return fmt.Errorf("invalid region for shell export: %s", region)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(rwDir, 0700); err != nil {
		return err
	}

	content := fmt.Sprintf("export AWS_PROFILE='%s'\n", profileName)
	content += "unset AWS_ACCESS_KEY_ID\n"
	content += "unset AWS_SECRET_ACCESS_KEY\n"
	content += "unset AWS_SESSION_TOKEN\n"
	if region != "" {
		content += fmt.Sprintf("export AWS_DEFAULT_REGION='%s'\n", region)
		content += fmt.Sprintf("export AWS_REGION='%s'\n", region)
	}

	return os.WriteFile(filepath.Join(rwDir, "env"), []byte(content), 0600)
}
