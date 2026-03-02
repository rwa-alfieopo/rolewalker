package aws

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"rolewalkers/internal/k8s"
	"rolewalkers/internal/utils"
	"strings"
)

// DatabaseManager handles database connection operations
type DatabaseManager struct {
	kubeManager     *KubeManager
	ssmManager      *SSMManager
	profileSwitcher *ProfileSwitcher
}

// DatabaseConfig holds configuration for a database connection
type DatabaseConfig struct {
	Environment string
	NodeType    string // read or write
	DBType      string // query or command
}

// NewDatabaseManagerWithDeps creates a new DatabaseManager with shared dependencies
func NewDatabaseManagerWithDeps(km *KubeManager, ssm *SSMManager, ps *ProfileSwitcher) *DatabaseManager {
	return &DatabaseManager{
		kubeManager:     km,
		ssmManager:      ssm,
		profileSwitcher: ps,
	}
}

// Connect spawns an interactive psql pod to connect to the database
func (dm *DatabaseManager) Connect(config DatabaseConfig) error {
	env := strings.ToLower(config.Environment)
	nodeType := strings.ToLower(config.NodeType)
	dbType := strings.ToLower(config.DBType)

	// Set defaults
	nodeType = cmp.Or(nodeType, "read")
	dbType = cmp.Or(dbType, "query")

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := dm.kubeManager.SwitchContextForEnvWithProfile(env, dm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get database endpoint from SSM
	fmt.Printf("Fetching database endpoint (%s/%s)...\n", dbType, nodeType)
	endpoint, err := dm.ssmManager.GetDatabaseEndpoint(env, nodeType, dbType)
	if err != nil {
		return fmt.Errorf("failed to get database endpoint: %w", err)
	}

	// Get database password from SSM
	fmt.Println("Fetching database credentials...")
	passwordPath := fmt.Sprintf("/%s/zenith/database/%s/db-zenithmaster-password", env, dbType)
	password, err := dm.ssmManager.GetParameter(passwordPath)
	if err != nil {
		return fmt.Errorf("failed to get database password: %w", err)
	}

	fmt.Printf("\nConnecting to database:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Database:    %s (%s node)\n", dbType, nodeType)
	fmt.Printf("  Endpoint:    %s\n", endpoint)
	fmt.Printf("  User:        zenithmaster\n")
	fmt.Println("\nStarting interactive psql session...")
	fmt.Println("(Type \\q or Ctrl+D to exit)")
	fmt.Println()

	return dm.runPsqlPod(endpoint, password)
}

// runPsqlPod spawns an interactive psql pod
func (dm *DatabaseManager) runPsqlPod(endpoint, password string) error {
	return k8s.RunPod(k8s.PodSpec{
		NamePrefix:  "psql",
		Image:       "postgres:15-alpine",
		Interactive: true,
		Command:     []string{"psql", "-h", endpoint, "-U", "zenithmaster", "-d", "postgres"},
		Env:         map[string]string{"PGPASSWORD": password},
	})
}



// BackupConfig holds configuration for database backup
type BackupConfig struct {
	Environment string
	OutputFile  string
	SchemaOnly  bool
}

// RestoreConfig holds configuration for database restore
type RestoreConfig struct {
	Environment string
	InputFile   string
	Clean       bool
}

// Backup performs a database backup using pg_dump via a temporary pod
func (dm *DatabaseManager) Backup(config BackupConfig) error {
	env := strings.ToLower(config.Environment)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := dm.kubeManager.SwitchContextForEnvWithProfile(env, dm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get database endpoint from SSM (use write node for backup to get latest data)
	fmt.Println("Fetching database endpoint...")
	endpoint, err := dm.ssmManager.GetDatabaseEndpoint(env, "write", "query")
	if err != nil {
		return fmt.Errorf("failed to get database endpoint: %w", err)
	}

	// Get database password from SSM
	fmt.Println("Fetching database credentials...")
	passwordPath := fmt.Sprintf("/%s/zenith/database/query/db-zenithmaster-password", env)
	password, err := dm.ssmManager.GetParameter(passwordPath)
	if err != nil {
		return fmt.Errorf("failed to get database password: %w", err)
	}

	fmt.Printf("\nStarting database backup:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Endpoint:    %s\n", endpoint)
	fmt.Printf("  Output:      %s\n", config.OutputFile)
	if config.SchemaOnly {
		fmt.Printf("  Mode:        Schema only\n")
	} else {
		fmt.Printf("  Mode:        Full backup (schema + data)\n")
	}
	fmt.Println("\nRunning pg_dump...")

	return dm.runPgDumpPod(endpoint, password, config)
}

// runPgDumpPod spawns a temporary pod to run pg_dump and captures output to file
func (dm *DatabaseManager) runPgDumpPod(endpoint, password string, config BackupConfig) (err error) {
	// Build pg_dump arguments
	pgDumpArgs := []string{
		"pg_dump",
		"-h", endpoint,
		"-U", "zenithmaster",
		"-d", "zenith",
	}
	if config.SchemaOnly {
		pgDumpArgs = append(pgDumpArgs, "--schema-only")
	}

	// Create output file
	outFile, err := os.Create(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() {
		if cerr := outFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()

	var stderr bytes.Buffer

	runErr := k8s.RunPod(k8s.PodSpec{
		NamePrefix: "pgdump",
		Image:      "postgres:15-alpine",
		Command:    pgDumpArgs,
		Env:        map[string]string{"PGPASSWORD": password},
		Operation:  "backup",
		Stdout:     outFile,
		Stderr:     &stderr,
	})

	if runErr != nil {
		outFile.Close()
		os.Remove(config.OutputFile)
		return fmt.Errorf("pg_dump failed: %w: %s", runErr, stderr.String())
	}

	// Get file size
	fileInfo, _ := os.Stat(config.OutputFile)
	size := fileInfo.Size()

	fmt.Printf("\n✓ Backup completed successfully!\n")
	fmt.Printf("  Output file: %s\n", config.OutputFile)
	fmt.Printf("  Size: %s\n", utils.FormatBytes(size))

	return nil
}

// Restore performs a database restore using psql via a temporary pod
func (dm *DatabaseManager) Restore(config RestoreConfig) error {
	env := strings.ToLower(config.Environment)

	// Check if input file exists
	if _, err := os.Stat(config.InputFile); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", config.InputFile)
	}

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := dm.kubeManager.SwitchContextForEnvWithProfile(env, dm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get database endpoint from SSM (use write node for restore)
	fmt.Println("Fetching database endpoint...")
	endpoint, err := dm.ssmManager.GetDatabaseEndpoint(env, "write", "query")
	if err != nil {
		return fmt.Errorf("failed to get database endpoint: %w", err)
	}

	// Get database password from SSM
	fmt.Println("Fetching database credentials...")
	passwordPath := fmt.Sprintf("/%s/zenith/database/query/db-zenithmaster-password", env)
	password, err := dm.ssmManager.GetParameter(passwordPath)
	if err != nil {
		return fmt.Errorf("failed to get database password: %w", err)
	}

	// Get file size for progress info
	fileInfo, _ := os.Stat(config.InputFile)

	fmt.Printf("\nStarting database restore:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Endpoint:    %s\n", endpoint)
	fmt.Printf("  Input:       %s (%s)\n", config.InputFile, utils.FormatBytes(fileInfo.Size()))
	if config.Clean {
		fmt.Printf("  Mode:        Clean (drop objects before recreating)\n")
	} else {
		fmt.Printf("  Mode:        Standard\n")
	}
	fmt.Println("\nRunning psql restore...")

	return dm.runPsqlRestorePod(endpoint, password, config)
}

// runPsqlRestorePod spawns a temporary pod to run psql and pipes SQL file to stdin
func (dm *DatabaseManager) runPsqlRestorePod(endpoint, password string, config RestoreConfig) error {
	// Build psql arguments
	psqlArgs := []string{
		"psql",
		"-h", endpoint,
		"-U", "zenithmaster",
		"-d", "zenith",
		"-v", "ON_ERROR_STOP=1",
	}

	// Open input file
	inFile, err := os.Open(config.InputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inFile.Close()

	var stdout, stderr bytes.Buffer

	runErr := k8s.RunPod(k8s.PodSpec{
		NamePrefix: "psql-restore",
		Image:      "postgres:15-alpine",
		Command:    psqlArgs,
		Env:        map[string]string{"PGPASSWORD": password},
		Operation:  "restore",
		Stdin:      inFile,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if runErr != nil {
		return fmt.Errorf("psql restore failed: %w: %s\n%s", runErr, stderr.String(), stdout.String())
	}

	fmt.Printf("\n✓ Restore completed successfully!\n")
	if stdout.Len() > 0 {
		fmt.Printf("\nOutput:\n%s\n", stdout.String())
	}

	return nil
}
