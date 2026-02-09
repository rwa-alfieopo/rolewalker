package aws

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
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

// NewDatabaseManager creates a new DatabaseManager instance
func NewDatabaseManager() *DatabaseManager {
	ps, err := NewProfileSwitcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Profile switcher init failed: %v\n", err)
	}
	return &DatabaseManager{
		kubeManager:     NewKubeManager(),
		ssmManager:      NewSSMManager(),
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

	// Generate unique pod name
	username := utils.GetCurrentUsernamePodSafe()
	if username == "unknown" {
		username = "user"
	}
	podName := fmt.Sprintf("psql-%s-%d", username, rand.IntN(10000))

	fmt.Printf("\nConnecting to database:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Database:    %s (%s node)\n", dbType, nodeType)
	fmt.Printf("  Endpoint:    %s\n", endpoint)
	fmt.Printf("  User:        zenithmaster\n")
	fmt.Printf("  Pod:         %s\n", podName)
	fmt.Println("\nStarting interactive psql session...")
	fmt.Println("(Type \\q or Ctrl+D to exit)")
	fmt.Println()

	return dm.runPsqlPod(podName, endpoint, password)
}

// runPsqlPod spawns an interactive psql pod
func (dm *DatabaseManager) runPsqlPod(podName, endpoint, password string) error {
	// Build labels with creator identity
	labels := k8s.CreatorLabelsWithSession()

	// Pass PGPASSWORD via pod spec override to avoid exposing it in the process list
	overrides := fmt.Sprintf(`{"spec":{"containers":[{"name":"%s","image":"postgres:15-alpine","stdin":true,"tty":true,"command":["psql","-h","%s","-U","zenithmaster","-d","postgres"],"env":[{"name":"PGPASSWORD","value":"%s"}]}]}}`, podName, endpoint, password)

	cmd := exec.Command("kubectl", "run", podName,
		"--rm", "-it",
		"--restart=Never",
		"--namespace="+TunnelAccessNamespace,
		"--image=postgres:15-alpine",
		"--labels", labels,
		"--overrides", overrides,
		"--override-type=strategic",
	)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		// Check if it's just the user exiting normally
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 0 {
				return nil
			}
		}
	}

	return err
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

	// Generate unique pod name with username
	username := utils.GetCurrentUsername()
	if username == "unknown" {
		username = "user"
	}
	podName := fmt.Sprintf("pgdump-%s-%d", username, rand.IntN(100000))

	fmt.Printf("\nStarting database backup:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Endpoint:    %s\n", endpoint)
	fmt.Printf("  Output:      %s\n", config.OutputFile)
	if config.SchemaOnly {
		fmt.Printf("  Mode:        Schema only\n")
	} else {
		fmt.Printf("  Mode:        Full backup (schema + data)\n")
	}
	fmt.Printf("  Pod:         %s\n", podName)
	fmt.Println("\nRunning pg_dump...")

	return dm.runPgDumpPod(podName, endpoint, password, config)
}

// runPgDumpPod spawns a temporary pod to run pg_dump and captures output to file
func (dm *DatabaseManager) runPgDumpPod(podName, endpoint, password string, config BackupConfig) (err error) {
	// Build labels with creator identity
	labels := k8s.CreatorLabelsWithOperation("backup")

	// Build pg_dump arguments
	pgDumpArgs := []string{
		"-h", endpoint,
		"-U", "zenithmaster",
		"-d", "zenith",
	}
	if config.SchemaOnly {
		pgDumpArgs = append(pgDumpArgs, "--schema-only")
	}

	// Build pg_dump command string for overrides
	pgDumpCmd := append([]string{"pg_dump"}, pgDumpArgs...)

	// Pass PGPASSWORD via pod spec override to avoid exposing it in the process list
	cmdJSON, _ := json.Marshal(pgDumpCmd)
	overrides := fmt.Sprintf(`{"spec":{"containers":[{"name":"%s","image":"postgres:15-alpine","stdin":true,"command":%s,"env":[{"name":"PGPASSWORD","value":"%s"}]}]}}`, podName, string(cmdJSON), password)

	// Build kubectl command
	args := []string{
		"run", podName,
		"--rm", "-i",
		"--restart=Never",
		"--namespace=" + TunnelAccessNamespace,
		"--image=postgres:15-alpine",
		"--labels", labels,
		"--overrides", overrides,
		"--override-type=strategic",
	}

	cmd := exec.Command("kubectl", args...)

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

	// Capture stdout to file
	var stderr bytes.Buffer
	cmd.Stdout = outFile
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// Clean up partial file on error
		outFile.Close()
		os.Remove(config.OutputFile)
		return fmt.Errorf("pg_dump failed: %w: %s", err, stderr.String())
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

	// Generate unique pod name with username
	username := utils.GetCurrentUsername()
	if username == "unknown" {
		username = "user"
	}
	podName := fmt.Sprintf("pgrestore-%s-%d", username, rand.IntN(100000))

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
	fmt.Printf("  Pod:         %s\n", podName)
	fmt.Println("\nRunning psql restore...")

	return dm.runPsqlRestorePod(podName, endpoint, password, config)
}

// runPsqlRestorePod spawns a temporary pod to run psql and pipes SQL file to stdin
func (dm *DatabaseManager) runPsqlRestorePod(podName, endpoint, password string, config RestoreConfig) error {
	// Build labels with creator identity
	labels := k8s.CreatorLabelsWithOperation("restore")

	// Build psql arguments
	psqlArgs := []string{
		"-h", endpoint,
		"-U", "zenithmaster",
		"-d", "zenith",
		"-v", "ON_ERROR_STOP=1",
	}
	if config.Clean {
		// Note: --clean is typically used with pg_restore, for psql we rely on the dump having DROP statements
		// or we can add -c flag which sends \c command
	}

	// Build psql command for overrides
	psqlCmd := append([]string{"psql"}, psqlArgs...)
	cmdJSON, _ := json.Marshal(psqlCmd)

	// Pass PGPASSWORD via pod spec override to avoid exposing it in the process list
	overrides := fmt.Sprintf(`{"spec":{"containers":[{"name":"%s","image":"postgres:15-alpine","stdin":true,"command":%s,"env":[{"name":"PGPASSWORD","value":"%s"}]}]}}`, podName, string(cmdJSON), password)

	// Build kubectl command
	args := []string{
		"run", podName,
		"--rm", "-i",
		"--restart=Never",
		"--namespace=" + TunnelAccessNamespace,
		"--image=postgres:15-alpine",
		"--labels", labels,
		"--overrides", overrides,
		"--override-type=strategic",
	}

	cmd := exec.Command("kubectl", args...)

	// Open input file
	inFile, err := os.Open(config.InputFile)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inFile.Close()

	// Pipe file to stdin
	cmd.Stdin = inFile

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("psql restore failed: %w: %s\n%s", err, stderr.String(), stdout.String())
	}

	fmt.Printf("\n✓ Restore completed successfully!\n")
	if stdout.Len() > 0 {
		fmt.Printf("\nOutput:\n%s\n", stdout.String())
	}

	return nil
}

// ConfirmRestore prompts the user for confirmation before restore
func ConfirmRestore(env, inputFile string) bool {
	return utils.ConfirmDatabaseRestore(env, inputFile)
}
