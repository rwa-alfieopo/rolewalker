package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rolewalkers/aws"
	"rolewalkers/internal/db"
	"rolewalkers/internal/utils"
	"rolewalkers/internal/web"
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
	dbRepo              *db.ConfigRepository
	configSync          *aws.ConfigSync
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

	// Initialize database repository (single shared instance)
	var dbRepo *db.ConfigRepository
	database, err := db.NewDB()
	if err == nil {
		dbRepo = db.NewConfigRepository(database)
	}

	// Create shared managers with injected dependencies
	km := aws.NewKubeManagerWithRepo(dbRepo)
	ssm := aws.NewSSMManagerWithRepo(dbRepo)

	tm, err := aws.NewTunnelManager()
	if err != nil {
		return nil, err
	}

	grpc := aws.NewGRPCManagerWithDeps(km, ps, dbRepo)
	dbMgr := aws.NewDatabaseManagerWithDeps(km, ssm, ps)
	redisMgr := aws.NewRedisManagerWithDeps(km, ssm, ps)
	mskMgr := aws.NewMSKManagerWithDeps(km, ssm, ps)
	maintMgr := aws.NewMaintenanceManagerWithRepo(dbRepo)
	scaleMgr := aws.NewScalingManagerWithDeps(km, ps, dbRepo)
	replMgr := aws.NewReplicationManagerWithRepo(dbRepo)

	// Initialize config sync
	var configSync *aws.ConfigSync
	if dbRepo != nil {
		cs, csErr := aws.NewConfigSync(dbRepo)
		if csErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ Config sync initialization failed: %v\n", csErr)
		} else {
			configSync = cs
		}
	}

	cli := &CLI{
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
		dbRepo:             dbRepo,
		configSync:         configSync,
	}

	// Auto-sync on first run: if config file exists but DB has no accounts/roles, import
	if configSync != nil && configSync.ConfigFileExists() && !configSync.HasExistingData() {
		result, err := configSync.SyncConfigToDB()
		if err == nil && result.Imported > 0 {
			fmt.Printf("✓ First run: imported %d profiles from ~/.aws/config into database\n", result.Imported)
			if len(result.Errors) > 0 {
				for _, e := range result.Errors {
					fmt.Printf("  ⚠ %s\n", e)
				}
			}
			fmt.Println("  Run 'rw config status' to review, or 'rw config generate' to let rw manage the config file")
			fmt.Println()
		}
	}

	return cli, nil
}

// Run executes the CLI with given arguments
func (c *CLI) Run(args []string) error {
	if len(args) < 1 {
		return c.showHelp()
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "list", "ls", "l":
		return c.listProfiles()
	case "switch", "use", "s":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rw switch <profile-name> [--no-kube]")
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
			return fmt.Errorf("usage: rw switch <profile-name> [--no-kube]")
		}
		return c.switchProfile(profileName, skipKube)
	case "login", "li":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rw login <profile-name>")
		}
		return c.login(cmdArgs[0])
	case "logout", "lo":
		if len(cmdArgs) < 1 {
			return fmt.Errorf("usage: rw logout <profile-name>")
		}
		return c.logout(cmdArgs[0])
	case "status", "st":
		return c.status()
	case "current", "c":
		return c.current()
	case "context", "ctx":
		return c.context(cmdArgs)
	case "kube", "k8s", "k":
		return c.kube(cmdArgs)
	case "db", "d":
		return c.db(cmdArgs)
	case "tunnel", "t":
		return c.tunnel(cmdArgs)
	case "port", "p":
		return c.port(cmdArgs)
	case "grpc", "g":
		return c.grpc(cmdArgs)
	case "redis", "r":
		return c.redis(cmdArgs)
	case "msk", "m":
		return c.msk(cmdArgs)
	case "maintenance", "mt":
		return c.maintenance(cmdArgs)
	case "scale", "sc":
		return c.scale(cmdArgs)
	case "replication", "rep":
		return c.replication(cmdArgs)
	case "keygen", "kg":
		return c.keygen(cmdArgs)
	case "ssm":
		return c.ssm(cmdArgs)
	case "set":
		return c.set(cmdArgs)
	case "config", "cfg":
		return c.config(cmdArgs)
	case "web", "w":
		return c.web(cmdArgs)
	case "help", "--help", "-h":
		return c.showHelp()
	case "version", "--version", "-v":
		return c.showVersion()
	case "example", "examples", "ex":
		return c.example()
	default:
		return fmt.Errorf("unknown command: %s\nRun 'rw help' for usage", command)
	}
}
func (c *CLI) example() error {
	examples := []string{
		"# Profile Management",
		"rw list                          # List all available AWS profiles",
		"rw switch dev                    # Switch to dev profile",
		"rw switch prod --no-kube         # Switch to prod without kubectl context",
		"rw login staging                 # Login to staging profile",
		"rw status                        # Show status of all profiles",
		"rw current                       # Show current active profile",
		"rw context                       # Show compact context info",
		"rw context --format short        # Output for shell prompts",
		"rw context --format json         # JSON output",
		"",
		"# Kubernetes",
		"rw kube                          # Show current kubectl context",
		"rw kube set-namespace            # Set default namespace",
		"rw kube pods                     # List pods in current namespace",
		"",
		"# Database",
		"rw db connect                    # Connect to database",
		"rw db backup                     # Create database backup",
		"rw db restore backup.sql         # Restore from backup",
		"",
		"# Tunnels & Port Forwarding",
		"rw tunnel start db               # Start database tunnel",
		"rw tunnel stop db                # Stop database tunnel",
		"rw port list                     # List available port forwards",
		"",
		"# Services",
		"rw grpc                          # Connect to gRPC service",
		"rw redis connect                 # Connect to Redis",
		"rw msk ui                        # Open Kafka UI",
		"",
		"# Maintenance & Scaling",
		"rw maintenance status            # Check maintenance mode",
		"rw maintenance on                # Enable maintenance mode",
		"rw scale list                    # List scalable resources",
		"rw scale deployment api 3        # Scale API deployment to 3 replicas",
		"",
		"# SSM Parameters",
		"rw ssm get /app/config           # Get SSM parameter",
		"rw ssm list /app/                # List SSM parameters",
		"",
		"# Replication",
		"rw replication status            # Check replication status",
		"rw replication switch primary    # Switch to primary",
		"",
		"# Shell Prompt",
		"rw set prompt                    # Enable prompt with all components",
		"rw set prompt time folder aws    # Pick specific components",
		"rw set prompt --reset            # Remove prompt customization",
		"rw set prompt --shell bash       # Force a specific shell",
		"",
		"# Config Management",
		"rw config status                 # Show sync status",
		"rw config sync                   # Import ~/.aws/config into database",
		"rw config generate               # Generate config from database",
		"rw config delete                 # Backup and remove config file",
	}

	fmt.Println("Examples:")
	fmt.Println()
	for _, example := range examples {
		fmt.Println(example)
	}
	return nil
}


func (c *CLI) showHelp() error {
	help := `rolewalkers (rw) - AWS Profile & SSO Manager

Usage: rw <command> [arguments]

Profile Management:
  list, ls, l             List all AWS profiles
  switch, use, s <profile>
                          Switch to a profile (updates default + kubectl context)
    --no-kube               Skip kubectl context switch
  login, li <profile>     SSO login for a profile
  logout, lo <profile>    SSO logout for a profile
  status, st              Show login status for all SSO profiles
  current, c              Show current active profile
  context, ctx [--format] Show compact context (profile, account, eks, namespace)
    --format short          Compact format for shell prompts
    --format json           JSON output

Kubernetes:
  kube, k <env>           Switch kubectl context to environment
  kube list               List available kubectl contexts
  kube set namespace      Interactively set default namespace

Port & Tunnel:
  port, p <svc> <env>     Get local port for a service/env
  port --list             List all port mappings
  tunnel, t start <svc> <env>
                          Start a tunnel to a service
  tunnel stop <svc> <env> Stop a specific tunnel
  tunnel stop --all       Stop all tunnels
  tunnel list             List active tunnels

Database:
  db, d connect <env>     Connect to database via interactive psql
    --write                 Connect to write node (default: read)
    --command               Connect to command database (default: query)
  db backup <env>         Backup database to local file
    --output, -o <file>     Output file path (required)
    --schema-only           Backup schema only, no data
  db restore <env>        Restore database from local file
    --input, -i <file>      Input file path (required)
    --clean                 Drop objects before recreating
    --yes, -y               Skip confirmation prompt

Redis:
  redis, r connect <env>  Connect to Redis cluster via interactive redis-cli

Kafka (MSK):
  msk, m ui <env>         Start Kafka UI for MSK cluster
    --port <port>           Local port (default: 8080)
  msk stop <env>          Stop the Kafka UI pod

Maintenance:
  maintenance, mt <env> --type <type> --enable|--disable
                          Toggle Fastly maintenance mode
  maintenance status <env>
                          Check maintenance mode status

Scaling:
  scale, sc <env> --preset <preset>
                          Scale all HPAs using a preset
  scale <env> --service <svc> --min <n> --max <n>
                          Scale a specific service's HPA
  scale list <env>        List HPAs and current scaling

Replication (Blue-Green):
  replication, rep status <env>
                          Show Blue-Green deployment status
  replication switch <id> [--yes]
                          Switchover a Blue-Green deployment
  replication create <env> --name <name> --source <cluster>
                          Create a new Blue-Green deployment
  replication delete <id> [--delete-target] [--yes]
                          Delete a Blue-Green deployment

gRPC:
  grpc, g <service> <env> Port-forward to a gRPC microservice
  grpc list               List available gRPC services

SSM Parameters:
  ssm get <path>          Get SSM parameter value
    --decrypt               Decrypt SecureString (default: enabled)
  ssm list <prefix>       List parameters under a path prefix

Configuration:
  config, cfg status      Show sync status between config file and database
  config sync             Import profiles from ~/.aws/config into database
  config generate         Generate ~/.aws/config from database
  config delete           Backup and delete ~/.aws/config (use DB only)
  set prompt [components] Configure shell prompt (time, folder, aws, k8s, git)
    --reset                 Remove prompt customization
    --shell <shell>         Override shell detection

Utilities:
  web, w                  Start web UI for account/role management
    --port <port>           Local port (default: 8080)
  keygen, kg [count]      Generate cryptographically secure API keys
  help, -h                Show this help message
  example, ex             Show usage examples

Tunnel Services: db, redis, elasticsearch, kafka, msk, rabbitmq, grpc
gRPC Services:   candidate, job, client, organisation, user, email, billing, core
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

	// Always switch kubectl context unless explicitly skipped
	if !skipKube {
		if err := c.kubeManager.SwitchContextForEnv(profileName); err != nil {
			fmt.Printf("⚠ Failed to switch kubectl context: %v\n", err)
		}
	}

	// Get current namespace
	namespace := c.kubeManager.GetCurrentNamespace()
	if namespace == "" {
		namespace = "default"
	}

	// Display the full context
	fmt.Println()
	c.showKubeContext(namespace)

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

	// After login, switch the default profile so bare AWS CLI commands
	// (e.g. "aws s3 ls") use the authenticated profile's credentials.
	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		fmt.Printf("⚠ Logged in but could not set default profile: %v\n", err)
		fmt.Printf("  Run 'rw switch %s' manually, or use --profile %s\n", profileName, profileName)
	}

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

func (c *CLI) context(args []string) error {
	format := "default"
	
	// Parse format flag
	for _, arg := range args {
		if arg == "--format" {
			continue
		}
		if arg == "short" || arg == "json" {
			format = arg
		}
	}
	
	// Get AWS profile info
	activeProfile := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()
	
	accountID := ""
	accountName := ""
	profiles, err := c.configManager.GetProfiles()
	if err == nil {
		for _, p := range profiles {
			if p.Name == activeProfile && p.IsSSO {
				accountID = p.SSOAccountID
				accountName = c.extractAccountName(p.Name)
				break
			}
		}
	}
	
	// Get Kubernetes info
	kubeContext := ""
	namespace := ""
	if ctx, err := c.kubeManager.GetCurrentContext(); err == nil {
		kubeContext = ctx
		// Extract just the cluster name (remove arn prefix if present)
		if strings.Contains(kubeContext, "/") {
			parts := strings.Split(kubeContext, "/")
			kubeContext = parts[len(parts)-1]
		}
	}
	
	ns := c.kubeManager.GetCurrentNamespace()
	if ns != "" {
		namespace = ns
	} else {
		namespace = "default"
	}
	
	// Output based on format
	switch format {
	case "short":
		// Compact format for shell prompts: profile|account|eks|namespace
		fmt.Printf("%s|%s|%s|%s\n", activeProfile, accountName, kubeContext, namespace)
		
	case "json":
		// JSON format — properly marshaled to prevent injection
		jsonOutput := map[string]string{
			"profile":      activeProfile,
			"account_name": accountName,
			"account_id":   accountID,
			"region":       region,
			"eks_cluster":  kubeContext,
			"namespace":    namespace,
		}
		if err := json.NewEncoder(os.Stdout).Encode(jsonOutput); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		
	default:
		// Human-readable format
		fmt.Printf("Profile:   %s\n", activeProfile)
		if accountName != "" {
			fmt.Printf("Account:   %s", accountName)
			if accountID != "" {
				fmt.Printf(" (%s)", accountID)
			}
			fmt.Println()
		}
		if region != "" {
			fmt.Printf("Region:    %s\n", region)
		}
		if kubeContext != "" {
			fmt.Printf("EKS:       %s\n", kubeContext)
			fmt.Printf("Namespace: %s\n", namespace)
		}
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
		return fmt.Errorf("usage: rw ssm <get|list> <path>\n\nSubcommands:\n  get <path>     Get parameter value\n  list <prefix>  List parameters under prefix\n\nExamples:\n  rw ssm get /dev/zenith/database/query/db-write-endpoint\n  rw ssm get /prod/zenith/redis/cluster-endpoint --decrypt\n  rw ssm list /dev/zenith/")
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
		return fmt.Errorf("usage: rw ssm get <path> [--decrypt]\n\nExamples:\n  rw ssm get /dev/zenith/database/query/db-write-endpoint\n  rw ssm get /prod/zenith/redis/cluster-endpoint")
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
		return fmt.Errorf("usage: rw ssm list <prefix>\n\nExamples:\n  rw ssm list /dev/zenith/\n  rw ssm list /prod/zenith/database/")
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
		return fmt.Errorf("usage: rw port <service> <env>\n       rw port --list\n\nServices: %s\nEnvironments: %s",
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
		return fmt.Errorf("usage: rw kube <env>\n       rw kube list\n       rw kube set namespace\n\nExamples:\n  rw kube dev              # Switch to dev EKS cluster context\n  rw kube prod             # Switch to prod EKS cluster context\n  rw kube list             # List all available contexts\n  rw kube set namespace    # Interactively set default namespace")
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

	// Handle set subcommand
	if subCmd == "set" {
		if len(args) < 2 {
			return fmt.Errorf("usage: rw kube set namespace")
		}
		if args[1] == "namespace" || args[1] == "ns" {
			return c.kubeSetNamespace()
		}
		return fmt.Errorf("unknown set option: %s\nUse: namespace", args[1])
	}

	// Otherwise treat as environment name
	env := subCmd
	
	// Get the proper AWS profile name for this environment
	profileName := c.kubeManager.GetProfileNameForEnv(env)
	
	// Switch AWS profile first
	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		return fmt.Errorf("failed to switch AWS profile: %w", err)
	}
	
	// Then switch kubectl context
	if err := c.kubeManager.SwitchContextForEnvWithProfile(env, c.profileSwitcher); err != nil {
		return err
	}

	// Get current namespace
	namespace := c.kubeManager.GetCurrentNamespace()
	if namespace == "" {
		namespace = "default"
	}

	// Display the full context
	fmt.Println()
	return c.showKubeContext(namespace)
}

func (c *CLI) kubeSetNamespace() error {
	// Get list of namespaces
	namespaces, err := c.kubeManager.ListNamespaces()
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		return fmt.Errorf("no namespaces found in current cluster")
	}

	// Interactive selection
	selectedNS, ok := utils.SelectFromList("Available namespaces:", namespaces)
	if !ok {
		fmt.Println("Namespace selection cancelled.")
		return nil
	}

	// Set the namespace
	if err := c.kubeManager.SetNamespace(selectedNS); err != nil {
		return fmt.Errorf("failed to set namespace: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Namespace set successfully!")
	fmt.Println()

	// Display confirmation with full context
	return c.showKubeContext(selectedNS)
}

func (c *CLI) showKubeContext(namespace string) error {
	activeProfile := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()
	
	fmt.Println("Current Context:")
	fmt.Println(strings.Repeat("-", 60))

	// AWS Profile
	fmt.Printf("AWS Profile:     %s\n", activeProfile)
	if region != "" {
		fmt.Printf("AWS Region:      %s\n", region)
	}

	// Get profile details (account ID and account name)
	profiles, err := c.configManager.GetProfiles()
	if err == nil {
		for _, p := range profiles {
			if p.Name == activeProfile && p.IsSSO {
				fmt.Printf("Account ID:      %s\n", p.SSOAccountID)
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
		fmt.Printf("Kube Namespace:  %s\n", namespace)
	} else {
		fmt.Printf("Kube Cluster:    (not configured)\n")
		fmt.Printf("Kube Namespace:  (not configured)\n")
	}

	return nil
}

func (c *CLI) tunnel(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw tunnel <start|stop|list> [service] [env]\n\nSubcommands:\n  start <service> <env>  Start a tunnel\n  stop <service> <env>   Stop a specific tunnel\n  stop --all             Stop all tunnels\n  list                   List active tunnels\n  cleanup                Remove stale tunnel entries\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage", aws.GetSupportedServices())
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
		return fmt.Errorf("usage: rw tunnel start <service> <env>\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage", aws.GetSupportedServices())
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
		return fmt.Errorf("usage: rw tunnel stop <service> <env>\n       rw tunnel stop --all")
	}

	service := args[0]
	env := args[1]

	return c.tunnelManager.Stop(service, env)
}

func (c *CLI) grpc(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw grpc <service> <env>\n       rw grpc list\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	// Handle list subcommand
	if args[0] == "list" || args[0] == "ls" {
		fmt.Print(c.grpcManager.ListServices())
		return nil
	}

	// Require service and environment
	if len(args) < 2 {
		return fmt.Errorf("usage: rw grpc <service> <env>\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	service := args[0]
	env := args[1]

	return c.grpcManager.Forward(service, env)
}

func (c *CLI) redis(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw redis connect <env>\n\nSubcommands:\n  connect <env>  Connect to Redis cluster via interactive redis-cli\n\nExamples:\n  rw redis connect dev   # Connect to dev Redis cluster\n  rw redis connect prod  # Connect to prod Redis cluster")
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
		return fmt.Errorf("usage: rw redis connect <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := args[0]
	return c.redisManager.Connect(env)
}

func (c *CLI) msk(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw msk <ui|stop> <env>\n\nSubcommands:\n  ui <env>    Start Kafka UI for MSK cluster\n  stop <env>  Stop the Kafka UI pod\n\nExamples:\n  rw msk ui dev              # Start Kafka UI on localhost:8080\n  rw msk ui prod --port 9090 # Start on custom port\n  rw msk stop dev            # Stop the Kafka UI pod")
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
		return fmt.Errorf("usage: rw msk ui <env> [--port <port>]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
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
		return fmt.Errorf("environment is required\n\nUsage: rw msk ui <env> [--port <port>]")
	}

	return c.mskManager.StartUI(env, port)
}

func (c *CLI) mskStop(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw msk stop <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	env := args[0]
	return c.mskManager.StopUI(env)
}

func (c *CLI) maintenance(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw maintenance <env> --type <api|pwa|all> --enable|--disable\n       rw maintenance status <env>\n\nSubcommands:\n  <env> --type <type> --enable   Enable maintenance mode\n  <env> --type <type> --disable  Disable maintenance mode\n  status <env>                   Check current maintenance status\n\nTypes: api, pwa, all\nEnvironments: snd, dev, sit, preprod, trg, prod\n\nRequires: FASTLY_API_TOKEN environment variable")
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
		return fmt.Errorf("usage: rw maintenance status <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod")
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
		return fmt.Errorf("environment is required\n\nUsage: rw maintenance <env> --type <api|pwa|all> --enable|--disable")
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

	// Production safety guard
	operation := "Enable Maintenance Mode"
	if disable {
		operation = "Disable Maintenance Mode"
	}
	if !utils.ConfirmProductionOperation(env, operation) {
		fmt.Println("Operation cancelled.")
		return nil
	}

	return c.maintenanceManager.Toggle(env, serviceType, enable)
}

func (c *CLI) scale(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw scale <env> --preset <preset>\n       rw scale <env> --service <svc> --min <n> --max <n>\n       rw scale list <env>\n\nPresets: normal (2/10), performance (10/50), minimal (1/3)\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage\n\nExamples:\n  rw scale preprod --preset performance\n  rw scale prod --preset normal\n  rw scale dev --service candidate --min 5 --max 10\n  rw scale list dev")
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

	// Production safety guard
	if !utils.ConfirmProductionOperation(env, fmt.Sprintf("Scale using preset '%s'", preset)) {
		fmt.Println("Operation cancelled.")
		return nil
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
		
		// Production safety guard for service scaling
		if !utils.ConfirmProductionOperation(env, fmt.Sprintf("Scale service '%s' to min=%d max=%d", service, minReplicas, maxReplicas)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
		
		return c.scalingManager.ScaleService(env, service, minReplicas, maxReplicas)
	}

	return fmt.Errorf("either --preset or --service with --min/--max is required")
}

func (c *CLI) scaleList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw scale list <env>")
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
		return fmt.Errorf("usage: rw db <connect|backup|restore> <env> [options]\n\nSubcommands:\n  connect <env>  Connect to database via interactive psql\n  backup <env>   Backup database to local file\n  restore <env>  Restore database from local file\n\nConnect flags:\n  --write, -w    Connect to write node (default: read)\n  --command, -c  Connect to command database (default: query)\n\nBackup flags:\n  --output, -o <file>  Output file path (required)\n  --schema-only        Backup schema only, no data\n\nRestore flags:\n  --input, -i <file>   Input file path (required)\n  --clean              Drop objects before recreating\n  --yes, -y            Skip confirmation prompt\n\nExamples:\n  rw db connect dev              # Connect to dev query database (read node)\n  rw db connect prod --write     # Connect to prod write node\n  rw db backup dev --output ./backup.sql\n  rw db backup dev --output ./schema.sql --schema-only\n  rw db restore dev --input ./backup.sql\n  rw db restore dev --input ./backup.sql --clean --yes")
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
		return fmt.Errorf("usage: rw db connect <env> [--write] [--command]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
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
		return fmt.Errorf("environment is required\n\nUsage: rw db connect <env> [--write] [--command]")
	}

	return c.dbManager.Connect(config)
}

func (c *CLI) dbBackup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw db backup <env> --output <file> [--schema-only]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
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
		return fmt.Errorf("environment is required\n\nUsage: rw db backup <env> --output <file>")
	}

	if config.OutputFile == "" {
		return fmt.Errorf("--output is required\n\nUsage: rw db backup <env> --output <file>")
	}

	return c.dbManager.Backup(config)
}

func (c *CLI) dbRestore(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw db restore <env> --input <file> [--clean] [--yes]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
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
		return fmt.Errorf("environment is required\n\nUsage: rw db restore <env> --input <file>")
	}

	if config.InputFile == "" {
		return fmt.Errorf("--input is required\n\nUsage: rw db restore <env> --input <file>")
	}

	// Production safety guard
	if !skipConfirm {
		if !utils.ConfirmProductionOperation(config.Environment, "Database Restore") {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Confirmation prompt for restore operations
	if !skipConfirm {
		if !utils.ConfirmDatabaseRestore(config.Environment, config.InputFile) {
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
	pm := aws.NewPromptManager()
	return pm.DetectShell()
}

func (c *CLI) initPowerShell(homeDir string) (err error) {
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
		if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
			return fmt.Errorf("failed to create profile directory: %w", err)
		}
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

func (c *CLI) replication(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw replication <status|switch|create|delete> [options]\n\nSubcommands:\n  status <env>           Show Blue-Green deployment status\n  switch <id> [--yes]    Switchover a deployment\n  create <env> --name <name> --source <cluster>\n                         Create a new Blue-Green deployment\n  delete <id> [--delete-target] [--yes]\n                         Delete a Blue-Green deployment\n\nExamples:\n  rw replication status dev\n  rw replication switch bgd-abc123\n  rw replication create dev --name my-bg --source prod-db-cluster\n  rw replication delete bgd-abc123 --yes")
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
		return fmt.Errorf("usage: rw replication status <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
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
		return fmt.Errorf("usage: rw replication switch <deployment-id> [--yes]\n\nExample:\n  rw replication switch bgd-abc123def456")
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
		if !utils.ConfirmReplicationSwitch(deploymentID, "(source)", "(target)") {
			fmt.Println("Switchover cancelled.")
			return nil
		}
	}

	return c.replicationManager.Switch("", deploymentID)
}

func (c *CLI) replicationCreate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw replication create <env> --name <name> --source <cluster>\n\nExample:\n  rw replication create dev --name my-blue-green --source prod-db-cluster")
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
		if !utils.ConfirmReplicationCreate(name, source) {
			fmt.Println("Creation cancelled.")
			return nil
		}
	}

	return c.replicationManager.Create(env, name, source)
}

func (c *CLI) replicationDelete(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw replication delete <deployment-id> [--delete-target] [--yes]\n\nExample:\n  rw replication delete bgd-abc123def456 --yes")
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
		if !utils.ConfirmReplicationDelete(deploymentID, deleteTarget) {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	return c.replicationManager.Delete(deploymentID, deleteTarget)
}

func (c *CLI) set(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw set <prompt> [options]\n\nSubcommands:\n  prompt [components...]  Configure shell prompt\n    Components: time, folder, aws, k8s, git\n    --reset               Remove rw prompt customization\n    --shell <shell>       Override shell detection (zsh, bash, powershell)\n\nExamples:\n  rw set prompt                          # Enable all components\n  rw set prompt time folder aws git      # Pick specific components\n  rw set prompt --reset                  # Remove prompt customization")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "prompt":
		return c.setPrompt(subArgs)
	default:
		return fmt.Errorf("unknown set subcommand: %s\nUse: prompt", subCmd)
	}
}

func (c *CLI) setPrompt(args []string) error {
	pm := aws.NewPromptManager()

	// Parse flags
	shell := pm.DetectShell()
	reset := false
	var components []aws.PromptComponent

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--reset", "--remove":
			reset = true
		case "--shell":
			if i+1 < len(args) {
				i++
				shell = strings.ToLower(args[i])
			} else {
				return fmt.Errorf("--shell requires a value (zsh, bash, powershell)")
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				comp := aws.PromptComponent(strings.ToLower(args[i]))
				valid := false
				for _, c := range aws.AllPromptComponents() {
					if comp == c {
						valid = true
						break
					}
				}
				if !valid {
					return fmt.Errorf("unknown prompt component: %s\nAvailable: time, folder, aws, k8s, git", args[i])
				}
				components = append(components, comp)
			}
		}
	}

	profilePath, err := pm.GetShellProfilePath(shell)
	if err != nil {
		return err
	}

	// Handle reset
	if reset {
		if err := pm.RemovePrompt(shell); err != nil {
			return fmt.Errorf("failed to remove prompt: %w", err)
		}
		fmt.Printf("✓ Removed rw prompt from: %s\n", profilePath)
		fmt.Printf("\nReload your shell:\n  source %s\n", profilePath)
		return nil
	}

	// Default to all components if none specified
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

func (c *CLI) config(args []string) error {
	if c.configSync == nil {
		return fmt.Errorf("database not initialized")
	}

	if len(args) < 1 {
		return fmt.Errorf("usage: rw config <status|sync|generate|delete>\n\nSubcommands:\n  status     Show sync status between ~/.aws/config and database\n  sync       Import/update profiles from ~/.aws/config into database\n  generate   Generate ~/.aws/config from database (rw manages the config)\n  delete     Backup and delete ~/.aws/config (use database only)")
	}

	switch args[0] {
	case "status":
		return c.configStatus()
	case "sync":
		return c.configSyncCmd()
	case "generate":
		return c.configGenerate()
	case "delete":
		return c.configDelete()
	default:
		return fmt.Errorf("unknown config subcommand: %s\nUse: status, sync, generate, delete", args[0])
	}
}

func (c *CLI) configStatus() error {
	hasConfig := c.configSync.ConfigFileExists()
	hasData := c.configSync.HasExistingData()

	fmt.Println("Config Sync Status:")
	fmt.Println(strings.Repeat("-", 50))

	if hasConfig {
		fmt.Printf("  ~/.aws/config:  ✓ exists (%s)\n", c.configSync.GetConfigPath())
	} else {
		fmt.Println("  ~/.aws/config:  ✗ not found")
	}

	if hasData {
		accounts, _ := c.dbRepo.GetAllAWSAccounts()
		roles, _ := c.dbRepo.GetAllAWSRoles()
		fmt.Printf("  Database:       ✓ %d accounts, %d roles\n", len(accounts), len(roles))
	} else {
		fmt.Println("  Database:       ✗ no accounts/roles")
	}

	if hasConfig && hasData {
		result, err := c.configSync.AnalyzeSync()
		if err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("  Sync analysis:")
		fmt.Printf("    New profiles to import: %d\n", result.Imported)
		fmt.Printf("    Profiles to update:     %d\n", result.Updated)
		fmt.Printf("    Already in sync:        %d\n", result.Skipped)

		if result.Imported > 0 || result.Updated > 0 {
			fmt.Println()
			fmt.Println("  Run 'rw config sync' to synchronize")
		} else {
			fmt.Println()
			fmt.Println("  ✓ Database is in sync with config file")
			fmt.Println("  You can run 'rw config delete' to remove the config file")
			fmt.Println("  and let rw manage it via 'rw config generate'")
		}
	} else if hasConfig && !hasData {
		fmt.Println()
		fmt.Println("  Config file found but database is empty.")
		fmt.Println("  Run 'rw config sync' to import profiles into the database.")
	} else if !hasConfig && hasData {
		fmt.Println()
		fmt.Println("  ✓ Running from database (no config file)")
		fmt.Println("  Run 'rw config generate' when you need ~/.aws/config for AWS CLI")
	}

	return nil
}

func (c *CLI) configSyncCmd() error {
	if !c.configSync.ConfigFileExists() {
		return fmt.Errorf("~/.aws/config not found, nothing to sync")
	}

	result, err := c.configSync.SyncConfigToDB()
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Println("Config Sync Results:")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("  Imported: %d\n", result.Imported)
	fmt.Printf("  Updated:  %d\n", result.Updated)
	fmt.Printf("  Skipped:  %d\n", result.Skipped)

	if len(result.Errors) > 0 {
		fmt.Println()
		fmt.Println("  Errors:")
		for _, e := range result.Errors {
			fmt.Printf("    ⚠ %s\n", e)
		}
	}

	if result.Imported > 0 || result.Updated > 0 {
		fmt.Println()
		fmt.Println("✓ Database updated. You can now:")
		fmt.Println("  rw config delete     # Remove ~/.aws/config (backup created)")
		fmt.Println("  rw config generate   # Regenerate config from database anytime")
	}

	return nil
}

func (c *CLI) configGenerate() error {
	if !c.configSync.HasExistingData() {
		return fmt.Errorf("no accounts/roles in database. Run 'rw config sync' first")
	}

	if c.configSync.ConfigFileExists() {
		backupPath, err := c.configSync.BackupConfigFile()
		if err != nil {
			return fmt.Errorf("failed to backup existing config: %w", err)
		}
		fmt.Printf("  Backed up existing config to: %s\n", backupPath)
	}

	if err := c.configSync.WriteAWSConfig(); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	fmt.Printf("✓ Generated ~/.aws/config from database\n")
	fmt.Printf("  Path: %s\n", c.configSync.GetConfigPath())
	return nil
}

func (c *CLI) configDelete() error {
	if !c.configSync.ConfigFileExists() {
		fmt.Println("~/.aws/config doesn't exist, nothing to delete")
		return nil
	}

	if !c.configSync.HasExistingData() {
		return fmt.Errorf("database has no accounts/roles. Run 'rw config sync' first before deleting the config file")
	}

	backupPath, err := c.configSync.BackupConfigFile()
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("  Backed up to: %s\n", backupPath)

	if !utils.ConfirmAction("Delete ~/.aws/config? (rw will generate it when needed)") {
		fmt.Println("Cancelled.")
		return nil
	}

	if err := c.configSync.DeleteConfigFile(); err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}

	fmt.Println("✓ Deleted ~/.aws/config")
	fmt.Println("  rw will generate it automatically when switching profiles")
	fmt.Println("  Or run 'rw config generate' to recreate it manually")
	return nil
}

func (c *CLI) web(args []string) error {
	port := 8080
	
	// Parse --port flag
	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			p, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i+1])
			}
			port = p
		}
	}

	// Create role switcher
	roleSwitcher, err := aws.NewRoleSwitcher(c.dbRepo)
	if err != nil {
		return fmt.Errorf("failed to create role switcher: %w", err)
	}

	// Create and start web server
	server := web.NewServer(port, c.dbRepo, roleSwitcher)
	return server.Start()
}

func RunCLI() {
	if err := runCLI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI() error {
	cli, err := NewCLI()
	if err != nil {
		return err
	}

	return cli.Run(os.Args[1:])
}
