package cli

import (
	"fmt"
	"rolewalkers/aws"
	"rolewalkers/internal/utils"
	"strings"
)

func (c *CLI) db(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw db <connect|backup|restore> <env> [options]\n\nSubcommands:\n  connect <env>  Connect to database via interactive psql\n  backup <env>   Backup database to local file\n  restore <env>  Restore database from local file\n\nConnect flags:\n  --write, -w       Connect to write node (default: read)\n  --command, -c     Connect to command database (default: query)\n  --readonly, --ro  Connect as read-only user (IAM auth)\n  --admin           Connect as admin user (IAM auth)\n  --iam             Force IAM authentication with master user\n\nBackup flags:\n  --output, -o <file>  Output file path (required)\n  --schema-only        Backup schema only, no data\n\nRestore flags:\n  --input, -i <file>   Input file path (required)\n  --clean              Drop objects before recreating\n  --yes, -y            Skip confirmation prompt\n\nExamples:\n  rw db connect dev              # Connect as zenithmaster (password)\n  rw db connect dev --readonly   # Connect as zenith-ro (IAM auth)\n  rw db connect prod --admin     # Connect as zenith-admin (IAM auth)\n  rw db connect prod --write --command  # Write node, command DB\n  rw db backup dev --output ./backup.sql\n  rw db restore dev --input ./backup.sql --clean --yes")
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
	config := aws.DatabaseConfig{
		NodeType: "read",
		DBType:   "query",
	}

	for _, arg := range args {
		switch arg {
		case "--write", "-w":
			config.NodeType = "write"
		case "--command", "-c":
			config.DBType = "command"
		case "--readonly", "--ro":
			config.Role = "readonly"
			config.UseIAM = true
		case "--admin":
			config.Role = "admin"
			config.UseIAM = true
			config.NodeType = "write" // admin typically needs write access
		case "--iam":
			config.UseIAM = true
		default:
			if !strings.HasPrefix(arg, "-") {
				config.Environment = arg
			}
		}
	}

	if config.Environment == "" {
		// Interactive environment picker
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		config.Environment = picked
	}

	return c.dbManager.Connect(config)
}

func (c *CLI) dbBackup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw db backup <env> --output <file> [--schema-only]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	fs := ParseFlags(args)
	config := aws.BackupConfig{
		Environment: fs.Arg(0),
		OutputFile:  fs.String("output", fs.String("o", "")),
		SchemaOnly:  fs.Bool("schema-only"),
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

	fs := ParseFlags(args)
	config := aws.RestoreConfig{
		Environment: fs.Arg(0),
		InputFile:   fs.String("input", fs.String("i", "")),
		Clean:       fs.Bool("clean"),
	}
	skipConfirm := fs.Bool("yes") || fs.Bool("y")

	if config.Environment == "" {
		return fmt.Errorf("environment is required\n\nUsage: rw db restore <env> --input <file>")
	}

	if config.InputFile == "" {
		return fmt.Errorf("--input is required\n\nUsage: rw db restore <env> --input <file>")
	}

	if !skipConfirm {
		if !utils.ConfirmProductionOperation(config.Environment, "Database Restore") {
			fmt.Println("Operation cancelled.")
			return nil
		}
		if !utils.ConfirmDatabaseRestore(config.Environment, config.InputFile) {
			fmt.Println("Restore cancelled.")
			return nil
		}
	}

	return c.dbManager.Restore(config)
}
