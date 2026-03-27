package main

import (
	"fmt"
	"os"
	"os/exec"
	"rolewalkers/tray"
)

func main() {
	// If --background flag is passed, re-exec ourselves without it and detach.
	for _, arg := range os.Args[1:] {
		if arg == "--background" || arg == "-bg" {
			daemonize()
			return
		}
	}

	tray.Run()
}

// daemonize re-launches the current executable without --background,
// detached from the terminal, and writes the PID to ~/.rolewalkers/tray.pid.
func daemonize() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find executable: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(exe)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	// SysProcAttr for detaching is set in the platform-specific file.
	tray.SetDetached(cmd)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start background process: %v\n", err)
		os.Exit(1)
	}

	// Write PID file
	if err := tray.WritePIDFile(cmd.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write PID file: %v\n", err)
	}

	fmt.Printf("✓ rw-tray started in background (PID %d)\n", cmd.Process.Pid)
}
