package cli

import (
	"fmt"
	"rolewalkers/internal/utils"
	"strings"
)

// --- Maintenance ---

func (c *CLI) maintenance(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw maintenance <env> --type <api|pwa|all> --enable|--disable\n       rw maintenance status <env>\n\nSubcommands:\n  <env> --type <type> --enable   Enable maintenance mode\n  <env> --type <type> --disable  Disable maintenance mode\n  status <env>                   Check current maintenance status\n\nTypes: api, pwa, all\nEnvironments: snd, dev, sit, preprod, trg, prod\n\nRequires: FASTLY_API_TOKEN environment variable")
	}

	if args[0] == "status" {
		return c.maintenanceStatus(args[1:])
	}

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
	fs := ParseFlags(args)
	env := fs.Arg(0)
	serviceType := fs.String("type", fs.String("t", ""))
	enable := fs.Bool("enable")
	disable := fs.Bool("disable")

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

// --- Scaling ---

func (c *CLI) scale(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw scale <env> --preset <preset>\n       rw scale <env> --service <svc> --min <n> --max <n>\n       rw scale list <env>\n\nPresets: normal (2/10), performance (10/50), minimal (1/3)\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage\n\nExamples:\n  rw scale preprod --preset performance\n  rw scale prod --preset normal\n  rw scale dev --service candidate --min 5 --max 10\n  rw scale list dev")
	}

	if args[0] == "list" || args[0] == "ls" {
		return c.scaleList(args[1:])
	}

	fs := ParseFlags(args)
	env := fs.Arg(0)
	preset := fs.String("preset", fs.String("p", ""))
	service := fs.String("service", fs.String("s", ""))

	if env == "" {
		return fmt.Errorf("environment is required")
	}

	if preset != "" {
		if !utils.ConfirmProductionOperation(env, fmt.Sprintf("Scale using preset '%s'", preset)) {
			fmt.Println("Operation cancelled.")
			return nil
		}
		return c.scalingManager.Scale(env, preset)
	}

	if service != "" {
		minReplicas, err := fs.Int("min", -1)
		if err != nil {
			return fmt.Errorf("invalid --min value")
		}
		maxReplicas, err := fs.Int("max", -1)
		if err != nil {
			return fmt.Errorf("invalid --max value")
		}
		if minReplicas < 0 || maxReplicas < 0 {
			return fmt.Errorf("--min and --max are required when using --service")
		}

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

	output, err := c.scalingManager.ListHPAs(args[0])
	if err != nil {
		return err
	}

	fmt.Print(output)
	return nil
}

// --- Replication ---

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

	output, err := c.replicationManager.Status(args[0])
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

	fs := ParseFlags(args)
	deploymentID := fs.Arg(0)
	skipConfirm := fs.Bool("yes") || fs.Bool("y")

	if deploymentID == "" {
		return fmt.Errorf("deployment identifier is required")
	}

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

	fs := ParseFlags(args)
	env := fs.Arg(0)
	name := fs.String("name", fs.String("n", ""))
	source := fs.String("source", fs.String("s", ""))
	skipConfirm := fs.Bool("yes") || fs.Bool("y")

	if env == "" {
		return fmt.Errorf("environment is required")
	}
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if source == "" {
		return fmt.Errorf("--source is required")
	}

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

	fs := ParseFlags(args)
	deploymentID := fs.Arg(0)
	deleteTarget := fs.Bool("delete-target")
	skipConfirm := fs.Bool("yes") || fs.Bool("y")

	if deploymentID == "" {
		return fmt.Errorf("deployment identifier is required")
	}

	if !skipConfirm {
		if !utils.ConfirmReplicationDelete(deploymentID, deleteTarget) {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	return c.replicationManager.Delete(deploymentID, deleteTarget)
}
