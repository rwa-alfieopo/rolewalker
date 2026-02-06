package db

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

// InitDB initializes the database with default data
func InitDB() error {
	db, err := NewDB()
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	fmt.Println("✓ Database initialized successfully")
	return nil
}

// GetDBPath returns the path to the database file
func GetDBPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".rolewalkers", "config.db"), nil
}

// ShowConfig displays the current configuration from the database
func ShowConfig() error {
	db, err := NewDB()
	if err != nil {
		return err
	}
	defer db.Close()

	repo := NewConfigRepository(db)

	// Show environments
	fmt.Println("\n=== Environments ===")
	envs, err := repo.GetAllEnvironments()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tREGION\tPROFILE\tCLUSTER")
	fmt.Fprintln(w, "----\t------\t-------\t-------")
	for _, env := range envs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", env.Name, env.Region, env.AWSProfile, env.ClusterName)
	}
	w.Flush()

	// Show services
	fmt.Println("\n=== Services ===")
	services, err := repo.GetAllServices()
	if err != nil {
		return err
	}

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tPORT\tDESCRIPTION")
	fmt.Fprintln(w, "----\t----\t----\t-----------")
	for _, svc := range services {
		if svc.ServiceType != "grpc-microservice" {
			desc := ""
			if svc.Description.Valid {
				desc = svc.Description.String
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", svc.Name, svc.ServiceType, svc.DefaultRemotePort, desc)
		}
	}
	w.Flush()

	// Show scaling presets
	fmt.Println("\n=== Scaling Presets ===")
	presets, err := repo.GetAllScalingPresets()
	if err != nil {
		return err
	}

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tMIN\tMAX\tDESCRIPTION")
	fmt.Fprintln(w, "----\t---\t---\t-----------")
	for _, preset := range presets {
		desc := ""
		if preset.Description.Valid {
			desc = preset.Description.String
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", preset.Name, preset.MinReplicas, preset.MaxReplicas, desc)
	}
	w.Flush()

	fmt.Println()
	return nil
}

// ResetDB drops and recreates the database
func ResetDB() error {
	dbPath, err := GetDBPath()
	if err != nil {
		return err
	}

	// Remove existing database
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing database: %w", err)
	}

	// Reinitialize
	return InitDB()
}
