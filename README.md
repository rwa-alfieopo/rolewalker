# rolewalkers

AWS Profile & SSO Manager - CLI tool for managing AWS profiles, SSO authentication, and Kubernetes operations.

## Features

- **SSO Login**: Authenticate via AWS SSO with browser-based login
- **Profile Switching**: Switch between AWS profiles, updating the default profile
- **AWS CLI Integration**: After switching, `aws` commands work without `--profile`
- **Kubernetes Integration**: Automatically switch kubectl contexts when switching profiles
- **Database Operations**: Connect, backup, and restore databases
- **Redis & MSK**: Connect to Redis clusters and manage Kafka UI
- **Maintenance Mode**: Toggle Fastly maintenance mode
- **Scaling**: Manage HPA scaling for services
- **Tunneling**: Port-forward to various services

## Installation

### Build from source

```bash
go build -o rwcli cmd/rwcli/main.go
```

## Usage

### CLI (rwcli)

```bash
# List all profiles
rwcli list

# Switch to a profile (updates default + kubectl context)
rwcli switch zenith-dev
rwcli switch zenith-dev --no-kube  # Skip kubectl context switch

# SSO login
rwcli login zenith-dev

# SSO logout
rwcli logout zenith-dev

# Show current profile
rwcli current

# Show SSO login status
rwcli status

# Kubernetes operations
rwcli kube dev              # Switch kubectl context
rwcli kube list             # List contexts

# Database operations
rwcli db connect dev        # Connect to database
rwcli db backup dev --output ./backup.sql
rwcli db restore dev --input ./backup.sql

# Redis operations
rwcli redis connect dev

# MSK operations
rwcli msk ui dev            # Start Kafka UI
rwcli msk stop dev          # Stop Kafka UI

# Maintenance mode
rwcli maintenance dev --type api --enable
rwcli maintenance status dev

# Scaling
rwcli scale preprod --preset performance
rwcli scale list dev

# Tunneling
rwcli tunnel start db dev
rwcli tunnel list

# gRPC port forwarding
rwcli grpc candidate dev
rwcli grpc list

# SSM parameters
rwcli ssm get /dev/zenith/database/query/db-write-endpoint
rwcli ssm list /dev/zenith/

# Generate API keys
rwcli keygen
rwcli keygen 5
```

### Shell Integration (PowerShell)

Add to your PowerShell profile (`$PROFILE`):

```powershell
function rw {
    param([string]$profile)
    rwcli switch $profile
    $env:AWS_PROFILE = $profile
}

# Usage: rw zenith-dev
```

### Shell Integration (Bash/Zsh)

Add to your `.bashrc` or `.zshrc`:

```bash
rw() {
    rwcli switch "$1"
    export AWS_PROFILE="$1"
}

# Usage: rw zenith-dev
```

## How It Works

1. **Profile Switching**: Updates the `[default]` section in `~/.aws/config` with the selected profile's settings
2. **SSO Login**: Uses `aws sso login` under the hood for browser-based authentication
3. **Region Handling**: The default region is set from the switched profile

After switching profiles, AWS CLI commands work without specifying `--profile`:

```bash
rwcli switch zenith-dev
aws s3 ls  # Uses zenith-dev profile
```

## Project Structure

```
rolewalkers/
├── aws/                 # AWS config and operations
│   ├── config.go        # Config file parsing
│   ├── sso.go           # SSO operations
│   ├── profile_switcher.go
│   ├── kubernetes.go    # Kubernetes operations
│   ├── database.go      # Database operations
│   ├── redis.go         # Redis operations
│   ├── msk.go           # MSK/Kafka operations
│   ├── maintenance.go   # Maintenance mode
│   ├── scaling.go       # HPA scaling
│   ├── tunnel.go        # Port forwarding
│   ├── grpc.go          # gRPC operations
│   └── ssm.go           # SSM parameter operations
├── cli/                 # CLI implementation
│   └── cli.go
├── cmd/rwcli/           # CLI entry point
│   └── main.go
└── main.go              # Main entry point
```

## Requirements

- Go 1.25+
- AWS CLI v2 (for SSO login)
- kubectl (for Kubernetes operations)
- psql (for database operations)
- redis-cli (for Redis operations)
