//go:build windows

package tray

import (
	"os/exec"
	"syscall"
)

// SetDetached configures the command to run detached from the terminal on Windows.
func SetDetached(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
