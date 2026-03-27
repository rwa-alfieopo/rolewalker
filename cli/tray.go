package cli

import (
	"fmt"
	"os"
	"os/exec"
	"rolewalkers/tray"
)

func (c *CLI) trayCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw tray <start|stop|status|restart>\n\nSubcommands:\n  start    Start the system tray app in the background\n  stop     Stop the running tray app\n  status   Check if the tray app is running\n  restart  Restart the tray app")
	}

	switch args[0] {
	case "start":
		return c.trayStart()
	case "stop":
		return c.trayStop()
	case "status":
		return c.trayStatus()
	case "restart":
		c.trayStop()
		return c.trayStart()
	default:
		return fmt.Errorf("unknown tray subcommand: %s\nUse: start, stop, status, restart", args[0])
	}
}

func (c *CLI) trayStart() error {
	if running, pid := tray.IsRunning(); running {
		return fmt.Errorf("tray is already running (PID %d)\nUse 'rw tray restart' to restart", pid)
	}

	// Find the rw-tray binary — check common locations
	exe := findTrayBinary()
	if exe == "" {
		return fmt.Errorf("rw-tray binary not found\nInstall it with: make build-tray && make install")
	}

	cmd := exec.Command(exe, "--background")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (c *CLI) trayStop() error {
	if err := tray.StopRunning(); err != nil {
		fmt.Printf("⚠ %v\n", err)
		return nil
	}
	fmt.Println("✓ Tray stopped")
	return nil
}

func (c *CLI) trayStatus() error {
	running, pid := tray.IsRunning()
	if running {
		fmt.Printf("✓ Tray is running (PID %d)\n", pid)
	} else {
		fmt.Println("✗ Tray is not running")
		fmt.Println("  Start it with: rw tray start")
	}
	return nil
}

// findTrayBinary looks for the rw-tray executable in common locations.
func findTrayBinary() string {
	candidates := []string{
		"rw-tray", // in PATH
	}

	// Check next to the current executable
	if exe, err := os.Executable(); err == nil {
		dir := exe[:len(exe)-len("rw")]
		candidates = append([]string{dir + "rw-tray"}, candidates...)
	}

	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}

	return ""
}
