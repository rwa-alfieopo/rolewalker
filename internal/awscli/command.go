package awscli

import (
	"os/exec"
	"runtime"
)

// CreateCommand creates an OS-compatible AWS CLI command
// On Windows, it wraps the command with cmd.exe
// On Unix-like systems, it executes directly
func CreateCommand(args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// On Windows, use cmd.exe to properly handle the AWS CLI
		cmdArgs := append([]string{"/C", "aws"}, args...)
		return exec.Command("cmd", cmdArgs...)
	}
	// On Unix-like systems (Linux, macOS), execute directly
	return exec.Command("aws", args...)
}

// CreateKubectlCommand creates a kubectl command
// Provided for consistency with AWS CLI command creation
func CreateKubectlCommand(args ...string) *exec.Cmd {
	return exec.Command("kubectl", args...)
}
