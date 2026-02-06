# rolewalkers

AWS Profile & SSO Manager - CLI and GUI tool for managing AWS profiles and SSO authentication.

## Features

- **SSO Login**: Authenticate via AWS SSO with browser-based login
- **Profile Switching**: Switch between AWS profiles, updating the default profile
- **AWS CLI Integration**: After switching, `aws` commands work without `--profile`
- **GUI & CLI**: Use the desktop app or command-line interface

## Installation

### Build from source

```bash
cd rolewalkers
wails3 build
```

### CLI only

```bash
go build -o rwcli.exe ./cmd/rwcli
```

## Usage

### GUI

Run `rolewalkers.exe` to open the desktop application.

### CLI (rwcli)

```bash
# List all profiles
rwcli list

# Switch to a profile (updates default)
rwcli switch zenith-dev

# SSO login
rwcli login zenith-dev

# SSO logout
rwcli logout zenith-dev

# Show current profile
rwcli current

# Show SSO login status
rwcli status

# Export environment variables
rwcli export powershell
rwcli export bash

# Show current AWS env vars
rwcli env
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
├── aws/                 # AWS config and SSO handling
│   ├── config.go        # Config file parsing
│   ├── sso.go           # SSO operations
│   └── profile_switcher.go
├── cli/                 # CLI implementation
│   └── cli.go
├── cmd/rwcli/           # CLI entry point
│   └── main.go
├── services/            # Wails services for GUI
│   └── aws_service.go
├── frontend/            # Svelte UI
└── main.go              # Main entry point (GUI + CLI)
```

## Requirements

- Go 1.21+
- AWS CLI v2 (for SSO login)
- Node.js 18+ (for building frontend)
