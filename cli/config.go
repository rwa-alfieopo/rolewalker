package cli

import (
	"fmt"
	"rolewalkers/internal/utils"
	"strings"
)

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
