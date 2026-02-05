package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"rolewalkers/aws"
	"runtime"
	"strings"
)

// CLI handles command-line operations
type CLI struct {
	configManager   *aws.ConfigManager
	ssoManager      *aws.SSOManager
	profileSwitcher *aws.ProfileSwitcher
	kubeManager     *aws.KubeManager
	tunnelManager   *aws.TunnelManager
}

// NewCLI creates a new CLI instance
func NewCLI() (*CLI, error) {
	cm, err := aws.NewConfigManager()
	if err != nil {
		return nil, err
	}

	sm, err := aws.NewSSOManager()
	if err != nil {
		return nil, err
	}

	ps, err := aws.NewProfileSwitcher()
	if err != nil {
		return nil, err
	}

	km := aws.NewKubeManager()

	tm, err := aws.NewTunnelManager()
	if err != nil {
		return nil, err
	}

	return &CLI{
		configManager:   cm,
		ssoManager:      sm,
		profileSwitcher: ps,
		kubeManager:     km,
		tunnelManager:   tm,
	}, nil
}

// Run executes the CLI with given arguments
func (c *CLI) Run(args []string) error {
	if len(args) < 1 {
		return c.showHelp()
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "list", "ls":
		return c.listProfiles()
	case "switch", "use":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rwcli switch <profile-name> [--kube]")
		}
		// Parse --kube flag
		withKube := false
		profileName := ""
		for _, arg := range cmdArgs {
			if arg == "--kube" || arg == "-k" {
				withKube = true
			} else if !strings.HasPrefix(arg, "-") {
				profileName = arg
			}
		}
		if profileName == "" {
			return fmt.Errorf("usage: rwcli switch <profile-name> [--kube]")
		}
		return c.switchProfile(profileName, withKube)
	case "login":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rwcli login <profile-name>")
		}
		return c.login(cmdArgs[0])
	case "logout":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rwcli logout <profile-name>")
		}
		return c.logout(cmdArgs[0])
	case "status":
		return c.status()
	case "current":
		return c.current()
	case "kube", "k8s":
		return c.kube(cmdArgs)
	case "tunnel":
		return c.tunnel(cmdArgs)
	case "port":
		return c.port(cmdArgs)
	case "gui", "--gui":
		return c.launchGUI()
	case "help", "--help", "-h":
		return c.showHelp()
	default:
		return fmt.Errorf("unknown command: %s\nRun 'rwcli help' for usage", command)
	}
}

func (c *CLI) showHelp() error {
	help := `rolewalkers (rwcli) - AWS Profile & SSO Manager

Usage: rwcli <command> [arguments]

Commands:
  list, ls              List all AWS profiles
  switch, use <profile> Switch to a profile (updates default)
    --kube, -k          Also switch kubectl context to matching EKS cluster
  login <profile>       SSO login for a profile
  logout <profile>      SSO logout for a profile
  status                Show login status for all SSO profiles
  current               Show current active profile
  kube <env>            Switch kubectl context to environment
  kube list             List available kubectl contexts
  port <svc> <env>      Get local port for a service/env
  port --list           List all port mappings
  tunnel start <svc> <env>  Start a tunnel to a service
  tunnel stop <svc> <env>   Stop a specific tunnel
  tunnel stop --all         Stop all tunnels
  tunnel list               List active tunnels
  gui, --gui            Launch the GUI application
  help                  Show this help message

Tunnel Services: db, redis, elasticsearch, kafka, msk, rabbitmq, grpc

Examples:
  rwcli list                     # List all profiles
  rwcli switch zenith-dev        # Switch AWS profile
  rwcli switch zenith-dev --kube # Switch AWS profile AND k8s context
  rwcli login my-sso-profile     # Login via SSO
  rwcli kube dev                 # Switch only k8s context
  rwcli port db dev              # Get database port for dev
  rwcli tunnel start db dev      # Start database tunnel to dev
  rwcli tunnel start redis prod  # Start redis tunnel to prod
  rwcli tunnel list              # Show active tunnels
  rwcli --gui                    # Open GUI
`
	fmt.Println(help)
	return nil
}

func (c *CLI) showVersion() error {
	fmt.Println("rolewalkers v1.0.0")
	return nil
}

func (c *CLI) listProfiles() error {
	profiles, err := c.configManager.GetProfiles()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Println("No AWS profiles found.")
		return nil
	}

	fmt.Println("AWS Profiles:")
	fmt.Println(strings.Repeat("-", 80))

	for _, p := range profiles {
		status := ""
		if p.IsActive {
			status = " [ACTIVE]"
		}

		ssoStatus := ""
		if p.IsSSO {
			if c.ssoManager.IsLoggedIn(p.Name) {
				ssoStatus = " (SSO: logged in)"
			} else {
				ssoStatus = " (SSO: not logged in)"
			}
		}

		fmt.Printf("  %s%s%s\n", p.Name, status, ssoStatus)

		if p.Region != "" {
			fmt.Printf("    Region: %s\n", p.Region)
		}
		if p.IsSSO {
			fmt.Printf("    Account: %s | Role: %s\n", p.SSOAccountID, p.SSORoleName)
		}
	}

	return nil
}

func (c *CLI) switchProfile(profileName string, withKube bool) error {
	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		return err
	}

	fmt.Printf("✓ Switched to profile: %s\n", profileName)

	// Switch kubectl context if requested
	if withKube {
		if err := c.kubeManager.SwitchContextForEnv(profileName); err != nil {
			fmt.Printf("⚠ Failed to switch kubectl context: %v\n", err)
		} else {
			ctx, _ := c.kubeManager.GetCurrentContext()
			fmt.Printf("✓ Switched kubectl context: %s\n", ctx)
		}
	}

	// Show export hint
	fmt.Println("\nTo update your current shell session, run:")
	fmt.Printf("  PowerShell: $env:AWS_PROFILE = '%s'\n", profileName)
	fmt.Printf("  Bash/Zsh:   export AWS_PROFILE='%s'\n", profileName)

	return nil
}

func (c *CLI) login(profileName string) error {
	fmt.Printf("Initiating SSO login for profile: %s\n", profileName)
	fmt.Println("A browser window will open for authentication...")

	if err := c.ssoManager.Login(profileName); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	fmt.Printf("✓ Successfully logged in to: %s\n", profileName)
	return nil
}

func (c *CLI) logout(profileName string) error {
	if err := c.ssoManager.Logout(profileName); err != nil {
		return fmt.Errorf("logout failed: %w", err)
	}

	fmt.Printf("✓ Logged out from: %s\n", profileName)
	return nil
}

func (c *CLI) status() error {
	profiles, err := c.ssoManager.GetSSOProfiles()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Println("No SSO profiles configured.")
		return nil
	}

	fmt.Println("SSO Profile Status:")
	fmt.Println(strings.Repeat("-", 60))

	for _, p := range profiles {
		status := "✗ Not logged in"
		if c.ssoManager.IsLoggedIn(p.Name) {
			status = "✓ Logged in"
			if expiry, err := c.ssoManager.GetCredentialExpiry(p.Name); err == nil {
				status += fmt.Sprintf(" (expires: %s)", expiry.Format("15:04:05"))
			}
		}

		active := ""
		if p.IsActive {
			active = " [ACTIVE]"
		}

		fmt.Printf("  %s%s: %s\n", p.Name, active, status)
	}

	return nil
}

func (c *CLI) current() error {
	active := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()

	fmt.Printf("Active Profile: %s\n", active)
	if region != "" {
		fmt.Printf("Default Region: %s\n", region)
	}

	// Check environment variables
	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" {
		fmt.Printf("AWS_PROFILE env: %s\n", envProfile)
	}
	if envRegion := os.Getenv("AWS_DEFAULT_REGION"); envRegion != "" {
		fmt.Printf("AWS_DEFAULT_REGION env: %s\n", envRegion)
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
			// Mask sensitive values
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

func (c *CLI) launchGUI() error {
	fmt.Println("Use 'rwcli' without arguments or 'rwcli gui' to launch GUI")
	return nil
}

func (c *CLI) port(args []string) error {
	portConfig := aws.NewPortConfig()

	// Handle --list flag
	if len(args) > 0 && (args[0] == "--list" || args[0] == "-l") {
		fmt.Print(portConfig.ListAll())
		return nil
	}

	// Require service and environment
	if len(args) < 2 {
		return fmt.Errorf("usage: rwcli port <service> <env>\n       rwcli port --list\n\nServices: %s\nEnvironments: %s",
			portConfig.GetServices(), portConfig.GetEnvironments())
	}

	service := args[0]
	env := args[1]

	ports, err := portConfig.GetPort(service, env)
	if err != nil {
		return err
	}

	// Output just the port(s)
	for i, p := range ports {
		if i > 0 {
			fmt.Print("/")
		}
		fmt.Print(p)
	}
	fmt.Println()

	return nil
}

func (c *CLI) kube(args []string) error {
	// Handle no args - show help
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli kube <env>\n       rwcli kube list\n\nExamples:\n  rwcli kube dev     # Switch to dev EKS cluster context\n  rwcli kube prod    # Switch to prod EKS cluster context\n  rwcli kube list    # List all available contexts")
	}

	subCmd := args[0]

	// Handle list subcommand
	if subCmd == "list" || subCmd == "ls" {
		output, err := c.kubeManager.ListContextsFormatted()
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	}

	// Handle current subcommand
	if subCmd == "current" {
		ctx, err := c.kubeManager.GetCurrentContext()
		if err != nil {
			return err
		}
		fmt.Printf("Current kubectl context: %s\n", ctx)
		return nil
	}

	// Otherwise treat as environment name
	env := subCmd
	if err := c.kubeManager.SwitchContextForEnv(env); err != nil {
		return err
	}

	ctx, _ := c.kubeManager.GetCurrentContext()
	fmt.Printf("✓ Switched kubectl context: %s\n", ctx)
	return nil
}

func (c *CLI) tunnel(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli tunnel <start|stop|list> [service] [env]\n\nSubcommands:\n  start <service> <env>  Start a tunnel\n  stop <service> <env>   Stop a specific tunnel\n  stop --all             Stop all tunnels\n  list                   List active tunnels\n  cleanup                Remove stale tunnel entries\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage", aws.GetSupportedServices())
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "start":
		return c.tunnelStart(subArgs)
	case "stop":
		return c.tunnelStop(subArgs)
	case "list", "ls":
		fmt.Print(c.tunnelManager.List())
		return nil
	case "cleanup":
		return c.tunnelManager.CleanupStale()
	default:
		return fmt.Errorf("unknown tunnel subcommand: %s\nUse: start, stop, list, cleanup", subCmd)
	}
}

func (c *CLI) tunnelStart(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: rwcli tunnel start <service> <env>\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage", aws.GetSupportedServices())
	}

	service := args[0]
	env := args[1]

	// Parse optional flags for database tunnels
	config := aws.TunnelConfig{
		Service:     service,
		Environment: env,
		NodeType:    "read",
		DBType:      "query",
	}

	// Parse additional flags
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--write", "-w":
			config.NodeType = "write"
		case "--command", "-c":
			config.DBType = "command"
		}
	}

	return c.tunnelManager.Start(config)
}

func (c *CLI) tunnelStop(args []string) error {
	// Handle --all flag
	if len(args) > 0 && (args[0] == "--all" || args[0] == "-a") {
		return c.tunnelManager.StopAll()
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: rwcli tunnel stop <service> <env>\n       rwcli tunnel stop --all")
	}

	service := args[0]
	env := args[1]

	return c.tunnelManager.Stop(service, env)
}

func (c *CLI) initShell(args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Auto-detect shell based on OS and environment
	shell := c.detectShell()

	// Allow override
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
	// Check SHELL env var (Unix)
	if shell := os.Getenv("SHELL"); shell != "" {
		if strings.Contains(shell, "zsh") {
			return "zsh"
		}
		if strings.Contains(shell, "bash") {
			return "bash"
		}
	}

	// Check PSModulePath (PowerShell indicator)
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}

	// Default based on OS
	if runtime.GOOS == "windows" {
		return "powershell"
	}

	return "bash"
}

func (c *CLI) initPowerShell(homeDir string) error {
	// PowerShell profile paths
	profilePaths := []string{
		filepath.Join(homeDir, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1"),
		filepath.Join(homeDir, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1"),
	}

	// Find existing profile or use first path
	var profilePath string
	for _, p := range profilePaths {
		if _, err := os.Stat(p); err == nil {
			profilePath = p
			break
		}
	}
	if profilePath == "" {
		profilePath = profilePaths[0]
		// Create directory if needed
		os.MkdirAll(filepath.Dir(profilePath), 0755)
	}

	// Check if already installed
	content, _ := os.ReadFile(profilePath)
	if strings.Contains(string(content), "# rolewalkers") {
		fmt.Println("✓ Shell integration already installed")
		fmt.Println("  Restart your terminal or run: . $PROFILE")
		return nil
	}

	// Append function
	funcCode := `

# rolewalkers - AWS Profile Switcher
function rw {
    param([Parameter(Position=0)][string]$profile)
    if (-not $profile) {
        rwcli list
        return
    }
    $result = rwcli switch $profile 2>&1
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
    (rwcli list 2>$null | Select-String '^\s+(\S+)' -AllMatches).Matches | 
        ForEach-Object { $_.Groups[1].Value } | 
        Where-Object { $_ -like "$wordToComplete*" }
}
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer f.Close()

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

func (c *CLI) initBash(homeDir string) error {
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
        rwcli list
        return
    fi
    if rwcli switch "$1"; then
        export AWS_PROFILE="$1"
        echo "✓ Switched to: $1"
    fi
}

# Tab completion for rw
_rw_completions() {
    local profiles=$(rwcli list 2>/dev/null | grep -oP '^\s+\K\S+')
    COMPREPLY=($(compgen -W "$profiles" -- "${COMP_WORDS[1]}"))
}
complete -F _rw_completions rw
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer f.Close()

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

func (c *CLI) initZsh(homeDir string) error {
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
        rwcli list
        return
    fi
    if rwcli switch "$1"; then
        export AWS_PROFILE="$1"
        echo "✓ Switched to: $1"
    fi
}

# Tab completion for rw
_rw() {
    local profiles=($(rwcli list 2>/dev/null | grep -oP '^\s+\K\S+'))
    _describe 'profile' profiles
}
compdef _rw rw
`

	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open profile: %w", err)
	}
	defer f.Close()

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

// RunCLI is the entry point for CLI mode
func RunCLI() {
	cli, err := NewCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
