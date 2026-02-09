package cli

import (
	"fmt"
	"rolewalkers/aws"
)

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

Tunnel Services: ` + aws.DefaultServices + `
gRPC Services:   ` + aws.DefaultGRPCServices + `
`
	fmt.Println(help)
	return nil
}

func (c *CLI) showVersion() error {
	fmt.Println("rolewalkers v1.0.0")
	return nil
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
