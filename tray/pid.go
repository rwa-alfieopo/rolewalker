package tray

import (
	"fmt"
	"os"
	"rolewalkers/internal/utils"
	"strconv"
	"strings"
	"syscall"
)

const pidFileName = "tray.pid"

// WritePIDFile writes the given PID to ~/.rolewalkers/tray.pid.
func WritePIDFile(pid int) error {
	return utils.WriteRoleWalkersFile(pidFileName, []byte(strconv.Itoa(pid)))
}

// ReadPID reads the PID from the PID file. Returns 0 if not found.
func ReadPID() int {
	data, err := utils.ReadRoleWalkersFile(pidFileName)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// RemovePIDFile removes the PID file.
func RemovePIDFile() {
	dir, err := utils.RoleWalkersDir()
	if err != nil {
		return
	}
	os.Remove(fmt.Sprintf("%s/%s", dir, pidFileName))
}

// IsRunning checks if the tray process is still alive.
func IsRunning() (bool, int) {
	pid := ReadPID()
	if pid == 0 {
		return false, 0
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, pid
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		RemovePIDFile()
		return false, pid
	}

	return true, pid
}

// StopRunning sends SIGTERM to the running tray process.
func StopRunning() error {
	running, pid := IsRunning()
	if !running {
		RemovePIDFile()
		return fmt.Errorf("tray is not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("could not find process %d: %w", pid, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("could not stop process %d: %w", pid, err)
	}

	RemovePIDFile()
	return nil
}
