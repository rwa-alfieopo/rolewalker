package cli

import (
	"fmt"
	"os"
	"path/filepath"
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	shell := c.detectShell()
	if len(args) > 0 {
		shell = strings.ToLower(args[0])
	}

	fmt.Printf("Detected shell: %s\n", shell)

	switch shell {
	case "powershell", "pwsh":
		return c.initPowerShell(homeDir)
	case "bash":
		return c.initBash(homeDir)
	case "zsh":
		return c.initZsh(homeDir)
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: powershell, bash, zsh", shell)
	}
}

func (c *CLI) detectShell() string {
	pm := aws.NewPromptManager()
	return pm.DetectShell()
}

func (c *CLI) initPowerShell(homeDir string) (err error) {
	profilePaths := []string{
		filepath.Join(homeDir, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(homeDir, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}

	var profilePath string
	for _, p := range profilePaths {
		if _, err := os.Stat(p); err == nil {
			profilePath = p
			break
		}
	}
	if profilePath == "" {
		profilePath = profilePaths[0]
		if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
			return fmt.Errorf("failed to create profile directory: %w", err)
		}
	}

	content, _ := os.ReadFile(profilePath)
	if strings.Contains(string(content), "# rolewalkers") {
		fmt.Println("✓ Shell integration already installed")
		fmt.Println("  Restart your terminal or run: . $PROFILE")
		return nil
	}

	funcCode := `

# rolewalkers - AWS Profile Switcher
function rw {
    param([Parameter(Position=0)][string]$profile)
    if (-not $profile) {
        rw list
        return
    }
    $result = rw switch $profile 2>&1
    if ($LASTEXITCODE -eq 0) {
        $env:AWS_PROFILE = $profile
        Write-Host "✓ Switched to: $profile" -ForegroundColor Green
    } else {
        Write-Host $result -ForegroundColor Red
    }
}

# Tab completion for rw
Register-ArgumentCompleter -CommandName rw -ParameterName profile -ScriptBlock {
    param($commandName, $parameterName, $wordToComplete)
    (rw list 2>$null | Select-String '^\s+(\S+)' -AllMatches).Matches | 
        ForEach-Object { $_.Groups[1].Value } | 
        Where-Object { $_ -like "$wordToComplete*" }
}
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()

	if _, err := f.WriteString(funcCode); err != nil {
		return fmt.Errorf("failed to write to profile: %w", err)
	}

	fmt.Printf("✓ Installed shell integration to: %s\n", profilePath)
	fmt.Println("\nTo activate now, run:")
	fmt.Println("  . $PROFILE")
	fmt.Println("\nThen use:")
	fmt.Println("  rw <profile-name>")

	return nil
}

func (c *CLI) initBash(homeDir string) (err error) {
	profilePath := filepath.Join(homeDir, ".bashrc")

	content, _ := os.ReadFile(profilePath)
	if strings.Contains(string(content), "# rolewalkers") {
		fmt.Println("✓ Shell integration already installed")
		fmt.Println("  Restart your terminal or run: source ~/.bashrc")
		return nil
	}

	funcCode := `

# rolewalkers - AWS Profile Switcher
rw() {
    if [ -z "$1" ]; then
        rw list
        return
    fi
    if rw switch "$1"; then
        export AWS_PROFILE="$1"
        echo "✓ Switched to: $1"
    fi
}

# Tab completion for rw
_rw_completions() {
    local profiles=$(rw list 2>/dev/null | grep -oP '^\s+\K\S+')
    COMPREPLY=($(compgen -W "$profiles" -- "${COMP_WORDS[1]}"))
}
complete -F _rw_completions rw
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()

	if _, err := f.WriteString(funcCode); err != nil {
		return fmt.Errorf("failed to write to profile: %w", err)
	}

	fmt.Printf("✓ Installed shell integration to: %s\n", profilePath)
	fmt.Println("\nTo activate now, run:")
	fmt.Println("  source ~/.bashrc")
	fmt.Println("\nThen use:")
	fmt.Println("  rw <profile-name>")

	return nil
}

func (c *CLI) initZsh(homeDir string) (err error) {
	profilePath := filepath.Join(homeDir, ".zshrc")

	content, _ := os.ReadFile(profilePath)
	if strings.Contains(string(content), "# rolewalkers") {
		fmt.Println("✓ Shell integration already installed")
		fmt.Println("  Restart your terminal or run: source ~/.zshrc")
		return nil
	}

	funcCode := `

# rolewalkers - AWS Profile Switcher
rw() {
    if [ -z "$1" ]; then
        rw list
        return
    fi
    if rw switch "$1"; then
        export AWS_PROFILE="$1"
        echo "✓ Switched to: $1"
    fi
}

# Tab completion for rw
_rw() {
    local profiles=($(rw list 2>/dev/null | grep -oP '^\s+\K\S+'))
    _describe 'profile' profiles
}
compdef _rw rw
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()

	if _, err := f.WriteString(funcCode); err != nil {
		return fmt.Errorf("failed to write to profile: %w", err)
	}

	fmt.Printf("✓ Installed shell integration to: %s\n", profilePath)
	fmt.Println("\nTo activate now, run:")
	fmt.Println("  source ~/.zshrc")
	fmt.Println("\nThen use:")
	fmt.Println("  rw <profile-name>")

	return nil
}
