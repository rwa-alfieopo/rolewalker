package cli

import (
	"fmt"
	"os"
	"rolewalkers/aws"
	"rolewalkers/internal/db"
	"strings"
)

// CLI handles command-line operations
type CLI struct {
	configManager      *aws.ConfigManager
	ssoManager         *aws.SSOManager
	profileSwitcher    *aws.ProfileSwitcher
	kubeManager        *aws.KubeManager
	tunnelManager      *aws.TunnelManager
	ssmManager         *aws.SSMManager
	grpcManager        *aws.GRPCManager
	dbManager          *aws.DatabaseManager
	redisManager       *aws.RedisManager
	mskManager         *aws.MSKManager
	maintenanceManager *aws.MaintenanceManager
	scalingManager     *aws.ScalingManager
	replicationManager *aws.ReplicationManager
	dbRepo             *db.ConfigRepository
	database           *db.DB
	configSync         *aws.ConfigSync
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
	var database *db.DB
	database, err = db.NewDB()
	if err == nil {
		dbRepo = db.NewConfigRepository(database)
	}

	// Create shared managers with injected dependencies
	km := aws.NewKubeManagerWithRepo(dbRepo)
	ssm := aws.NewSSMManagerWithRepo(dbRepo)

	tm, err := aws.NewTunnelManagerWithDeps(km, ssm, ps, dbRepo)
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
		configManager:      cm,
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
		database:           database,
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

// Close releases resources held by the CLI (e.g. database connections).
func (c *CLI) Close() {
	if c.database != nil {
		c.database.Close()
	}
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
		return c.switchCmd(cmdArgs)
	case "login", "li":
		return c.loginCmd(cmdArgs)
	case "logout", "lo":
		return c.logoutCmd(cmdArgs)
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

// switchCmd wraps the switch command with argument validation.
func (c *CLI) switchCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw switch <profile-name> [--no-kube]")
	}
	fs := ParseFlags(args)
	profileName := fs.Arg(0)
	if profileName == "" {
		return fmt.Errorf("usage: rw switch <profile-name> [--no-kube]")
	}
	skipKube := fs.Bool("no-kube") || fs.Bool("skip-kube")
	return c.switchProfile(profileName, skipKube)
}

// loginCmd wraps the login command with argument validation.
func (c *CLI) loginCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw login <profile-name>")
	}
	return c.login(args[0])
}

// logoutCmd wraps the logout command with argument validation.
func (c *CLI) logoutCmd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw logout <profile-name>")
	}
	return c.logout(args[0])
}

// extractAccountName extracts a friendly account name from the profile name
func (c *CLI) extractAccountName(profileName string) string {
	name := strings.TrimPrefix(profileName, "zenith-")
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

// RunCLI is the main entry point called from cmd/rw/main.go.
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
	defer cli.Close()
	return cli.Run(os.Args[1:])
}
