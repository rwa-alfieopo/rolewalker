package aws

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeEnvFile writes AWS environment variables to ~/.rolewalkers/env
// so the shell can source it to pick up AWS_PROFILE automatically.
func writeEnvFile(profileName, region string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	rwDir := filepath.Join(homeDir, ".rolewalkers")
	if err := os.MkdirAll(rwDir, 0700); err != nil {
		return err
	}

	content := fmt.Sprintf("export AWS_PROFILE='%s'\n", profileName)
	if region != "" {
		content += fmt.Sprintf("export AWS_DEFAULT_REGION='%s'\n", region)
		content += fmt.Sprintf("export AWS_REGION='%s'\n", region)
	}

	return os.WriteFile(filepath.Join(rwDir, "env"), []byte(content), 0600)
}
