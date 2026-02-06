package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"rolewalkers/aws"
	"runtime"
	"strconv"
	"strings"
)

// CLI handles command-line operations
type CLI struct {
	configManager       *aws.ConfigManager
	ssoManager          *aws.SSOManager
	profileSwitcher     *aws.ProfileSwitcher
	kubeManager         *aws.KubeManager
	tunnelManager       *aws.TunnelManager
	ssmManager          *aws.SSMManager
	grpcManager         *aws.GRPCManager
	dbManager           *aws.DatabaseManager
	redisManager        *aws.RedisManager
	mskManager          *aws.MSKManager
	maintenanceManager  *aws.MaintenanceManager
	scalingManager      *aws.ScalingManager
	replicationManager  *aws.ReplicationManager
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

	ssm := aws.NewSSMManager()
	grpc := aws.NewGRPCManager()
	dbMgr := aws.NewDatabaseManager()
	redisMgr := aws.NewRedisManager()
	mskMgr := aws.NewMSKManager()
	maintMgr := aws.NewMaintenanceManager()
	scaleMgr := aws.NewScalingManager()
	replMgr := aws.NewReplicationManager()

	return &CLI{
		configManager:       cm,
		ssoManager:         sm,
		profileSwitcher:    ps,
		kubeManager:        km,
		tunnelManager:      tm,
		ssmManager:         ssm,
		grpcManager:        grpc,
		dbManager:          dbMgr,
		redisManager:       redisMgr,
		mskManager:         mskMgr,
		maintenanceManager: maintMgr,
		scalingManager:     scaleMgr,
		replicationManager: replMgr,
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
			return fmt.Errorf("usage: rwcli switch <profile-name> [--no-kube]")
		}
		// Parse --no-kube flag (kubectl context switches by default now)
		skipKube := false
		profileName := ""
		for _, arg := range cmdArgs {
			if arg == "--no-kube" || arg == "--skip-kube" {
				skipKube = true
			} else if !strings.HasPrefix(arg, "-") {
				profileName = arg
			}
		}
		if profileName == "" {
			return fmt.Errorf("usage: rwcli switch <profile-name> [--no-kube]")
		}
		return c.switchProfile(profileName, skipKube)
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
	case "db":
		return c.db(cmdArgs)
	case "tunnel":
		return c.tunnel(cmdArgs)
	case "port":
		return c.port(cmdArgs)
	case "grpc":
		return c.grpc(cmdArgs)
	case "redis":
		return c.redis(cmdArgs)
	case "msk":
		return c.msk(cmdArgs)
	case "maintenance":
		return c.maintenance(cmdArgs)
	case "scale":
		return c.scale(cmdArgs)
	case "replication":
		return c.replication(cmdArgs)
	case "keygen":
		return c.keygen(cmdArgs)
	case "ssm":
		return c.ssm(cmdArgs)
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
  switch, use <profile> Switch to a profile (updates default + kubectl context)
    --no-kube           Skip kubectl context switch
  login <profile>       SSO login for a profile
  logout <profile>      SSO logout for a profile
  status                Show login status for all SSO profiles
  current               Show current active profile
  kube <env>            Switch kubectl context to environment
  kube list             List available kubectl contexts
  port <svc> <env>      Get local port for a service/env
  port --list           List all port mappings
  db connect <env>      Connect to database via interactive psql
    --write             Connect to write node (default: read)
    --command           Connect to command database (default: query)
  db backup <env>       Backup database to local file
    --output, -o <file> Output file path (required)
    --schema-only       Backup schema only, no data
  db restore <env>      Restore database from local file
    --input, -i <file>  Input file path (required)
    --clean             Drop objects before recreating
    --yes, -y           Skip confirmation prompt
  redis connect <env>   Connect to Redis cluster via interactive redis-cli
  msk ui <env>          Start Kafka UI for MSK cluster
    --port <port>       Local port (default: 8080)
  msk stop <env>        Stop the Kafka UI pod
  maintenance <env> --type <type> --enable|--disable
                        Toggle Fastly maintenance mode
  maintenance status <env>
                        Check maintenance mode status
  scale <env> --preset <preset>
                        Scale all HPAs using a preset
  scale <env> --service <svc> --min <n> --max <n>
                        Scale a specific service's HPA
  scale list <env>      List HPAs and current scaling
  replication status <env>
                        Show Blue-Green deployment status
  replication switch <id> [--yes]
                        Switchover a Blue-Green deployment
  replication create <env> --name <name> --source <cluster>
                        Create a new Blue-Green deployment
  replication delete <id> [--delete-target] [--yes]
                        Delete a Blue-Green deployment
  tunnel start <svc> <env>  Start a tunnel to a service
  tunnel stop <svc> <env>   Stop a specific tunnel
  tunnel stop --all         Stop all tunnels
  tunnel list               List active tunnels
  grpc <service> <env>  Port-forward to a gRPC microservice
  grpc list             List available gRPC services
  ssm get <path>        Get SSM parameter value
    --decrypt           Decrypt SecureString (default: enabled)
  ssm list <prefix>     List parameters under a path prefix
  keygen [count]        Generate cryptographically secure API keys
  help                  Show this help message

Tunnel Services: db, redis, elasticsearch, kafka, msk, rabbitmq, grpc
gRPC Services: candidate, job, client, organisation, user, email, billing, core

Examples:
  rwcli list                     # List all profiles
  rwcli switch zenith-dev        # Switch AWS profile + k8s context
  rwcli switch zenith-dev --no-kube # Switch AWS profile only
  rwcli login my-sso-profile     # Login via SSO
  rwcli kube dev                 # Switch only k8s context
  rwcli port db dev              # Get database port for dev
  rwcli db connect dev           # Connect to dev query database (read node)
  rwcli db connect prod --write  # Connect to prod write node
  rwcli db connect prod --command # Connect to command database
  rwcli db backup dev --output ./backup.sql
  rwcli db backup dev --output ./schema.sql --schema-only
  rwcli db restore dev --input ./backup.sql
  rwcli db restore dev --input ./backup.sql --clean --yes
  rwcli redis connect dev        # Connect to dev Redis cluster
  rwcli redis connect prod       # Connect to prod Redis cluster
  rwcli msk ui dev               # Start Kafka UI on localhost:8080
  rwcli msk ui prod --port 9090  # Start Kafka UI on custom port
  rwcli msk stop dev             # Stop the Kafka UI pod
  rwcli maintenance dev --type api --enable   # Enable API maintenance
  rwcli maintenance prod --type all --disable # Disable all maintenance
  rwcli maintenance status dev                # Check maintenance status
  rwcli scale preprod --preset performance    # Scale up for performance testing
  rwcli scale prod --preset normal            # Reset to normal scaling
  rwcli scale dev --service candidate --min 5 --max 10  # Custom scaling
  rwcli scale list dev                        # List HPAs and scaling
  rwcli replication status dev                # Show Blue-Green deployments
  rwcli replication switch bgd-abc123 --yes   # Switchover deployment
  rwcli replication create dev --name my-bg --source prod-db-cluster
  rwcli replication delete bgd-abc123 --yes   # Delete deployment
  rwcli tunnel start db dev      # Start database tunnel to dev
  rwcli tunnel start redis prod  # Start redis tunnel to prod
  rwcli tunnel list              # Show active tunnels
  rwcli grpc candidate dev       # Forward localhost:5001 to candidate-microservice-grpc
  rwcli grpc job prod            # Forward localhost:5002 to job-microservice-grpc
  rwcli grpc list                # List all gRPC services and ports
  rwcli ssm get /dev/zenith/database/query/db-write-endpoint
  rwcli ssm list /dev/zenith/   # List all params under prefix
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

func (c *CLI) switchProfile(profileName string, skipKube bool) error {
	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		return err
	}

	fmt.Printf("✓ Switched to profile: %s\n", profileName)

	// Always switch kubectl context unless explicitly skipped
	if !skipKube {
		if err := c.kubeManager.SwitchContextForEnv(profileName); err != nil {
			fmt.Printf("⚠ Failed to switch kubectl context: %v\n", err)
		} else {
			ctx, _ := c.kubeManager.GetCurrentContext()
			fmt.Printf("✓ Switched kubectl context: %s\n", ctx)
		}
	}

	// Check if AWS_PROFILE env var is set - it overrides the config file
	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" && envProfile != profileName {
		fmt.Println("\n⚠ AWS_PROFILE environment variable is set and overrides the config.")
		fmt.Println("  Clear it for this terminal:")
		if runtime.GOOS == "windows" {
			fmt.Println("    Remove-Item Env:AWS_PROFILE")
		} else {
			fmt.Println("    unset AWS_PROFILE")
		}
		fmt.Println("  (New terminals will work automatically)")
	}

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

	fmt.Println("Current Context:")
	fmt.Println(strings.Repeat("-", 60))

	// AWS Profile
	fmt.Printf("AWS Profile:     %s\n", active)
	if region != "" {
		fmt.Printf("AWS Region:      %s\n", region)
	}

	// Get profile details (account ID and account name)
	profiles, err := c.configManager.GetProfiles()
	if err == nil {
		for _, p := range profiles {
			if p.Name == active && p.IsSSO {
				fmt.Printf("Account ID:      %s\n", p.SSOAccountID)
				// Account name is typically derived from the profile name or role
				accountName := c.extractAccountName(p.Name)
				if accountName != "" {
					fmt.Printf("Account Name:    %s\n", accountName)
				}
				break
			}
		}
	}

	// Kubernetes context
	kubeContext, err := c.kubeManager.GetCurrentContext()
	if err == nil && kubeContext != "" {
		fmt.Printf("Kube Cluster:    %s\n", kubeContext)
		
		// Get namespace (default to "default" if not set)
		namespace := c.kubeManager.GetCurrentNamespace()
		if namespace == "" {
			namespace = "default"
		}
		fmt.Printf("Kube Namespace:  %s\n", namespace)
	} else {
		fmt.Printf("Kube Cluster:    (not configured)\n")
		fmt.Printf("Kube Namespace:  (not configured)\n")
	}

	// Check environment variable overrides
	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" && envProfile != active {
		fmt.Printf("\n⚠ AWS_PROFILE env override: %s\n", envProfile)
	}
	if envRegion := os.Getenv("AWS_DEFAULT_REGION"); envRegion != "" && envRegion != region {
		fmt.Printf("⚠ AWS_DEFAULT_REGION env override: %s\n", envRegion)
	}

	return nil
}

// extractAccountName extracts a friendly account name from the profile name
func (c *CLI) extractAccountName(profileName string) string {
	// Remove common prefixes like "zenith-"
	name := strings.TrimPrefix(profileName, "zenith-")
	
	// Capitalize first letter
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	
	return name
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

func (c *CLI) ssm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli ssm <get|list> <path>\n\nSubcommands:\n  get <path>     Get parameter value\n  list <prefix>  List parameters under prefix\n\nExamples:\n  rwcli ssm get /dev/zenith/database/query/db-write-endpoint\n  rwcli ssm get /prod/zenith/redis/cluster-endpoint --decrypt\n  rwcli ssm list /dev/zenith/")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "get":
		return c.ssmGet(subArgs)
	case "list", "ls":
		return c.ssmList(subArgs)
	default:
		return fmt.Errorf("unknown ssm subcommand: %s\nUse: get, list", subCmd)
	}
}

func (c *CLI) ssmGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli ssm get <path> [--decrypt]\n\nExamples:\n  rwcli ssm get /dev/zenith/database/query/db-write-endpoint\n  rwcli ssm get /prod/zenith/redis/cluster-endpoint")
	}

	path := args[0]

	// --decrypt is already the default behavior in SSMManager
	value, err := c.ssmManager.GetParameter(path)
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}

func (c *CLI) ssmList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli ssm list <prefix>\n\nExamples:\n  rwcli ssm list /dev/zenith/\n  rwcli ssm list /prod/zenith/database/")
	}

	prefix := args[0]

	params, err := c.ssmManager.ListParameters(prefix)
	if err != nil {
		return err
	}

	if len(params) == 0 {
		fmt.Printf("No parameters found under: %s\n", prefix)
		return nil
	}

	fmt.Printf("Parameters under %s:\n", prefix)
	for _, p := range params {
		fmt.Printf("  %s\n", p)
	}

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

func (c *CLI) grpc(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli grpc <service> <env>\n       rwcli grpc list\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	// Handle list subcommand
	if args[0] == "list" || args[0] == "ls" {
		fmt.Print(c.grpcManager.ListServices())
		return nil
	}

	// Require service and environment
	if len(args) < 2 {
		return fmt.Errorf("usage: rwcli grpc <service> <env>\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	service := args[0]
	env := args[1]

	return c.grpcManager.Forward(service, env)
}

func (c *CLI) redis(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli redis connect <env>\n\nSubcommands:\n  connect <env>  Connect to Redis cluster via interactive redis-cli\n\nExamples:\n  rwcli redis connect dev   # Connect to dev Redis cluster\n  rwcli redis connect prod  # Connect to prod Redis cluster")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "connect":
		return c.redisConnect(subArgs)
	default:
		return fmt.Errorf("unknown redis subcommand: %s\nUse: connect", subCmd)
	}
}

func (c *CLI) redisConnect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli redis connect <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := args[0]
	return c.redisManager.Connect(env)
}

func (c *CLI) msk(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli msk <ui|stop> <env>\n\nSubcommands:\n  ui <env>    Start Kafka UI for MSK cluster\n  stop <env>  Stop the Kafka UI pod\n\nExamples:\n  rwcli msk ui dev              # Start Kafka UI on localhost:8080\n  rwcli msk ui prod --port 9090 # Start on custom port\n  rwcli msk stop dev            # Stop the Kafka UI pod")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "ui":
		return c.mskUI(subArgs)
	case "stop":
		return c.mskStop(subArgs)
	default:
		return fmt.Errorf("unknown msk subcommand: %s\nUse: ui, stop", subCmd)
	}
}

func (c *CLI) mskUI(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli msk ui <env> [--port <port>]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := ""
	port := 8080

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			if i+1 < len(args) {
				i++
				p, err := strconv.Atoi(args[i])
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("invalid port: %s", args[i])
				}
				port = p
			} else {
				return fmt.Errorf("--port requires a value")
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				env = args[i]
			}
		}
	}

	if env == "" {
		return fmt.Errorf("environment is required\n\nUsage: rwcli msk ui <env> [--port <port>]")
	}

	return c.mskManager.StartUI(env, port)
}

func (c *CLI) mskStop(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli msk stop <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := args[0]
	return c.mskManager.StopUI(env)
}

func (c *CLI) maintenance(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli maintenance <env> --type <api|pwa|all> --enable|--disable\n       rwcli maintenance status <env>\n\nSubcommands:\n  <env> --type <type> --enable   Enable maintenance mode\n  <env> --type <type> --disable  Disable maintenance mode\n  status <env>                   Check current maintenance status\n\nTypes: api, pwa, all\nEnvironments: snd, dev, sit, preprod, trg, prod\n\nRequires: FASTLY_API_TOKEN environment variable")
	}

	// Handle status subcommand
	if args[0] == "status" {
		return c.maintenanceStatus(args[1:])
	}

	// Parse toggle command
	return c.maintenanceToggle(args)
}

func (c *CLI) maintenanceStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli maintenance status <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod")
	}

	env := args[0]
	statuses, err := c.maintenanceManager.Status(env)
	if err != nil {
		return err
	}

	fmt.Printf("Maintenance Mode Status for %s:\n", env)
	fmt.Println(strings.Repeat("-", 50))

	for _, s := range statuses {
		status := "✗ Disabled"
		if s.Enabled {
			status = "✓ Enabled"
		}
		if s.Error != "" {
			status = fmt.Sprintf("⚠ Error: %s", s.Error)
		}

		fmt.Printf("  %s (%s): %s\n", strings.ToUpper(s.ServiceType), s.ServiceName, status)
	}

	return nil
}

func (c *CLI) maintenanceToggle(args []string) error {
	env := ""
	serviceType := ""
	enable := false
	disable := false

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type", "-t":
			if i+1 < len(args) {
				i++
				serviceType = args[i]
			} else {
				return fmt.Errorf("--type requires a value (api, pwa, all)")
			}
		case "--enable":
			enable = true
		case "--disable":
			disable = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				env = args[i]
			}
		}
	}

	if env == "" {
		return fmt.Errorf("environment is required\n\nUsage: rwcli maintenance <env> --type <api|pwa|all> --enable|--disable")
	}

	if serviceType == "" {
		return fmt.Errorf("--type is required (api, pwa, all)")
	}

	if !enable && !disable {
		return fmt.Errorf("either --enable or --disable is required")
	}

	if enable && disable {
		return fmt.Errorf("cannot use both --enable and --disable")
	}

	return c.maintenanceManager.Toggle(env, serviceType, enable)
}

func (c *CLI) scale(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli scale <env> --preset <preset>\n       rwcli scale <env> --service <svc> --min <n> --max <n>\n       rwcli scale list <env>\n\nPresets: normal (2/10), performance (10/50), minimal (1/3)\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage\n\nExamples:\n  rwcli scale preprod --preset performance\n  rwcli scale prod --preset normal\n  rwcli scale dev --service candidate --min 5 --max 10\n  rwcli scale list dev")
	}

	// Handle list subcommand
	if args[0] == "list" || args[0] == "ls" {
		return c.scaleList(args[1:])
	}

	// Parse arguments
	env := ""
	preset := ""
	service := ""
	minReplicas := -1
	maxReplicas := -1

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--preset", "-p":
			if i+1 < len(args) {
				i++
				preset = args[i]
			} else {
				return fmt.Errorf("--preset requires a value")
			}
		case "--service", "-s":
			if i+1 < len(args) {
				i++
				service = args[i]
			} else {
				return fmt.Errorf("--service requires a value")
			}
		case "--min":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil || n < 0 {
					return fmt.Errorf("invalid --min value: %s", args[i])
				}
				minReplicas = n
			} else {
				return fmt.Errorf("--min requires a value")
			}
		case "--max":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil || n < 0 {
					return fmt.Errorf("invalid --max value: %s", args[i])
				}
				maxReplicas = n
			} else {
				return fmt.Errorf("--max requires a value")
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				env = args[i]
			}
		}
	}

	if env == "" {
		return fmt.Errorf("environment is required")
	}

	// Determine which scaling mode to use
	if preset != "" {
		// Preset mode - scale all HPAs
		return c.scalingManager.Scale(env, preset)
	}

	if service != "" {
		// Service mode - scale specific HPA
		if minReplicas < 0 || maxReplicas < 0 {
			return fmt.Errorf("--min and --max are required when using --service")
		}
		return c.scalingManager.ScaleService(env, service, minReplicas, maxReplicas)
	}

	return fmt.Errorf("either --preset or --service with --min/--max is required")
}

func (c *CLI) scaleList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli scale list <env>")
	}

	env := args[0]
	output, err := c.scalingManager.ListHPAs(env)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

func (c *CLI) db(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli db <connect|backup|restore> <env> [options]\n\nSubcommands:\n  connect <env>  Connect to database via interactive psql\n  backup <env>   Backup database to local file\n  restore <env>  Restore database from local file\n\nConnect flags:\n  --write, -w    Connect to write node (default: read)\n  --command, -c  Connect to command database (default: query)\n\nBackup flags:\n  --output, -o <file>  Output file path (required)\n  --schema-only        Backup schema only, no data\n\nRestore flags:\n  --input, -i <file>   Input file path (required)\n  --clean              Drop objects before recreating\n  --yes, -y            Skip confirmation prompt\n\nExamples:\n  rwcli db connect dev              # Connect to dev query database (read node)\n  rwcli db connect prod --write     # Connect to prod write node\n  rwcli db backup dev --output ./backup.sql\n  rwcli db backup dev --output ./schema.sql --schema-only\n  rwcli db restore dev --input ./backup.sql\n  rwcli db restore dev --input ./backup.sql --clean --yes")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "connect":
		return c.dbConnect(subArgs)
	case "backup":
		return c.dbBackup(subArgs)
	case "restore":
		return c.dbRestore(subArgs)
	default:
		return fmt.Errorf("unknown db subcommand: %s\nUse: connect, backup, restore", subCmd)
	}
}

func (c *CLI) dbConnect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli db connect <env> [--write] [--command]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	config := aws.DatabaseConfig{
		NodeType: "read",
		DBType:   "query",
	}

	// Parse arguments
	for _, arg := range args {
		switch arg {
		case "--write", "-w":
			config.NodeType = "write"
		case "--command", "-c":
			config.DBType = "command"
		default:
			if !strings.HasPrefix(arg, "-") {
				config.Environment = arg
			}
		}
	}

	if config.Environment == "" {
		return fmt.Errorf("environment is required\n\nUsage: rwcli db connect <env> [--write] [--command]")
	}

	return c.dbManager.Connect(config)
}

func (c *CLI) dbBackup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli db backup <env> --output <file> [--schema-only]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	config := aws.BackupConfig{}

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 < len(args) {
				i++
				config.OutputFile = args[i]
			} else {
				return fmt.Errorf("--output requires a file path")
			}
		case "--schema-only":
			config.SchemaOnly = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				config.Environment = args[i]
			}
		}
	}

	if config.Environment == "" {
		return fmt.Errorf("environment is required\n\nUsage: rwcli db backup <env> --output <file>")
	}

	if config.OutputFile == "" {
		return fmt.Errorf("--output is required\n\nUsage: rwcli db backup <env> --output <file>")
	}

	return c.dbManager.Backup(config)
}

func (c *CLI) dbRestore(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli db restore <env> --input <file> [--clean] [--yes]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	config := aws.RestoreConfig{}
	skipConfirm := false

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--input", "-i":
			if i+1 < len(args) {
				i++
				config.InputFile = args[i]
			} else {
				return fmt.Errorf("--input requires a file path")
			}
		case "--clean":
			config.Clean = true
		case "--yes", "-y":
			skipConfirm = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				config.Environment = args[i]
			}
		}
	}

	if config.Environment == "" {
		return fmt.Errorf("environment is required\n\nUsage: rwcli db restore <env> --input <file>")
	}

	if config.InputFile == "" {
		return fmt.Errorf("--input is required\n\nUsage: rwcli db restore <env> --input <file>")
	}

	// Confirmation prompt for restore operations
	if !skipConfirm {
		if !aws.ConfirmRestore(config.Environment, config.InputFile) {
			fmt.Println("Restore cancelled.")
			return nil
		}
	}

	return c.dbManager.Restore(config)
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

func (c *CLI) replication(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli replication <status|switch|create|delete> [options]\n\nSubcommands:\n  status <env>           Show Blue-Green deployment status\n  switch <id> [--yes]    Switchover a deployment\n  create <env> --name <name> --source <cluster>\n                         Create a new Blue-Green deployment\n  delete <id> [--delete-target] [--yes]\n                         Delete a Blue-Green deployment\n\nExamples:\n  rwcli replication status dev\n  rwcli replication switch bgd-abc123\n  rwcli replication create dev --name my-bg --source prod-db-cluster\n  rwcli replication delete bgd-abc123 --yes")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "status":
		return c.replicationStatus(subArgs)
	case "switch":
		return c.replicationSwitch(subArgs)
	case "create":
		return c.replicationCreate(subArgs)
	case "delete":
		return c.replicationDelete(subArgs)
	default:
		return fmt.Errorf("unknown replication subcommand: %s\nUse: status, switch, create, delete", subCmd)
	}
}

func (c *CLI) replicationStatus(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli replication status <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := args[0]
	output, err := c.replicationManager.Status(env)
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

func (c *CLI) replicationSwitch(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli replication switch <deployment-id> [--yes]\n\nExample:\n  rwcli replication switch bgd-abc123def456")
	}

	deploymentID := ""
	skipConfirm := false

	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			skipConfirm = true
		default:
			if !strings.HasPrefix(arg, "-") {
				deploymentID = arg
			}
		}
	}

	if deploymentID == "" {
		return fmt.Errorf("deployment identifier is required")
	}

	// Confirmation prompt
	if !skipConfirm {
		if !aws.ConfirmReplicationSwitch(deploymentID, "(source)", "(target)") {
			fmt.Println("Switchover cancelled.")
			return nil
		}
	}

	return c.replicationManager.Switch("", deploymentID)
}

func (c *CLI) replicationCreate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli replication create <env> --name <name> --source <cluster>\n\nExample:\n  rwcli replication create dev --name my-blue-green --source prod-db-cluster")
	}

	env := ""
	name := ""
	source := ""
	skipConfirm := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name", "-n":
			if i+1 < len(args) {
				i++
				name = args[i]
			} else {
				return fmt.Errorf("--name requires a value")
			}
		case "--source", "-s":
			if i+1 < len(args) {
				i++
				source = args[i]
			} else {
				return fmt.Errorf("--source requires a value")
			}
		case "--yes", "-y":
			skipConfirm = true
		default:
			if !strings.HasPrefix(args[i], "-") {
				env = args[i]
			}
		}
	}

	if env == "" {
		return fmt.Errorf("environment is required")
	}

	if name == "" {
		return fmt.Errorf("--name is required")
	}

	if source == "" {
		return fmt.Errorf("--source is required")
	}

	// Confirmation prompt
	if !skipConfirm {
		if !aws.ConfirmReplicationCreate(name, source) {
			fmt.Println("Creation cancelled.")
			return nil
		}
	}

	return c.replicationManager.Create(env, name, source)
}

func (c *CLI) replicationDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rwcli replication delete <deployment-id> [--delete-target] [--yes]\n\nExample:\n  rwcli replication delete bgd-abc123def456 --yes")
	}

	deploymentID := ""
	deleteTarget := false
	skipConfirm := false

	for _, arg := range args {
		switch arg {
		case "--delete-target":
			deleteTarget = true
		case "--yes", "-y":
			skipConfirm = true
		default:
			if !strings.HasPrefix(arg, "-") {
				deploymentID = arg
			}
		}
	}

	if deploymentID == "" {
		return fmt.Errorf("deployment identifier is required")
	}

	// Confirmation prompt
	if !skipConfirm {
		if !aws.ConfirmReplicationDelete(deploymentID, deleteTarget) {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	return c.replicationManager.Delete(deploymentID, deleteTarget)
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
