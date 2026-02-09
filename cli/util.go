package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *CLI) keygen(args []string) error {
	count := 1
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("invalid count: %s (must be a positive integer)", args[0])
		}
		count = n
	}

	for i := 0; i < count; i++ {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			return fmt.Errorf("failed to generate random key: %w", err)
		}
		fmt.Println(hex.EncodeToString(bytes))
	}

	return nil
}

func (c *CLI) export(args []string) error {
	shell := "powershell"
	if len(args) > 0 {
		shell = strings.ToLower(args[0])
	}

	active := c.configManager.GetActiveProfile()
	export, err := c.profileSwitcher.GenerateShellExport(active, shell)
	if err != nil {
		return err
	}

	fmt.Print(export)
	return nil
}

func (c *CLI) showEnv() error {
	envVars := []string{
		"AWS_PROFILE",
		"AWS_DEFAULT_REGION",
		"AWS_REGION",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
	}

	fmt.Println("Current AWS Environment Variables:")
	fmt.Println(strings.Repeat("-", 40))

	for _, v := range envVars {
		value := os.Getenv(v)
		if value != "" {
			if strings.Contains(v, "SECRET") || strings.Contains(v, "TOKEN") || strings.Contains(v, "KEY_ID") {
				if len(value) > 8 {
					value = value[:4] + "..." + value[len(value)-4:]
				} else {
					value = "****"
				}
			}
			fmt.Printf("  %s = %s\n", v, value)
		} else {
			fmt.Printf("  %s = (not set)\n", v)
		}
	}

	return nil
}
