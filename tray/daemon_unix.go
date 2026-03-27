//go:build !windows

package tray

import (
	"os/exec"
	"syscall"
)

// SetDetached configures the command to run detached from the terminal on Unix.
func SetDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
