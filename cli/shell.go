package cli

import (
	"fmt"
	"rolewalkers/aws"
	"strings"
)

func (c *CLI) set(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw set <prompt> [options]\n\nSubcommands:\n  prompt [components...]  Configure shell prompt\n    Components: time, folder, aws, k8s, git\n    --reset               Remove rw prompt customization\n    --shell <shell>       Override shell detection (zsh, bash, powershell)\n\nExamples:\n  rw set prompt                          # Enable all components\n  rw set prompt time folder aws git      # Pick specific components\n  rw set prompt --reset                  # Remove prompt customization")
	}

	switch args[0] {
	case "prompt":
		return c.setPrompt(args[1:])
	default:
		return fmt.Errorf("unknown set subcommand: %s\nUse: prompt", args[0])
	}
}

func (c *CLI) setPrompt(args []string) error {
	pm := aws.NewPromptManager()

	fs := ParseFlags(args)
	shell := fs.String("shell", pm.DetectShell())
	reset := fs.Bool("reset") || fs.Bool("remove")

	var components []aws.PromptComponent
	for _, name := range fs.Positional() {
		comp := aws.PromptComponent(strings.ToLower(name))
		valid := false
		for _, c := range aws.AllPromptComponents() {
			if comp == c {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown prompt component: %s\nAvailable: time, folder, aws, k8s, git", name)
		}
		components = append(components, comp)
	}

	profilePath, err := pm.GetShellProfilePath(shell)
	if err != nil {
		return err
	}

	if reset {
		if err := pm.RemovePrompt(shell); err != nil {
			return fmt.Errorf("failed to remove prompt: %w", err)
		}
		fmt.Printf("✓ Removed rw prompt from: %s\n", profilePath)
		fmt.Printf("\nReload your shell:\n  source %s\n", profilePath)
		return nil
	}

	if len(components) == 0 {
		components = aws.AllPromptComponents()
	}

	if err := pm.InstallPrompt(shell, components); err != nil {
		return fmt.Errorf("failed to install prompt: %w", err)
	}

	fmt.Printf("✓ Prompt installed to: %s\n", profilePath)
	fmt.Printf("  Shell:      %s\n", shell)
	fmt.Printf("  Components: ")
	for i, comp := range components {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(string(comp))
	}
	fmt.Println()
	fmt.Printf("\nReload your shell:\n  source %s\n", profilePath)
	return nil
}

func (c *CLI) initShell(args []string) error {
	// Deprecated: shell integration is now handled by 'rw set prompt'.
	return fmt.Errorf("'init-shell' has been removed. Use 'rw set prompt' instead")
}
