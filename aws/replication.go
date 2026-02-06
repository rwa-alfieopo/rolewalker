package aws

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ReplicationManager handles RDS Blue-Green deployment operations
type ReplicationManager struct {
	region string
}

// BlueGreenDeployment represents an RDS Blue-Green deployment
type BlueGreenDeployment struct {
	Identifier    string    `json:"BlueGreenDeploymentIdentifier"`
	Name          string    `json:"BlueGreenDeploymentName"`
	Source        string    `json:"Source"`
	Target        string    `json:"Target"`
	Status        string    `json:"Status"`
	StatusDetails string    `json:"StatusDetails,omitempty"`
	CreateTime    time.Time `json:"CreateTime"`
	Tasks         []struct {
		Name   string `json:"Name"`
		Status string `json:"Status"`
	} `json:"Tasks,omitempty"`
	SwitchoverDetails *struct {
		SourceMember string `json:"SourceMember"`
		TargetMember string `json:"TargetMember"`
		Status       string `json:"Status"`
	} `json:"SwitchoverDetails,omitempty"`
}

// BlueGreenDeploymentsResponse represents the AWS CLI response
type BlueGreenDeploymentsResponse struct {
	BlueGreenDeployments []BlueGreenDeployment `json:"BlueGreenDeployments"`
}

// NewReplicationManager creates a new ReplicationManager instance
func NewReplicationManager() *ReplicationManager {
	return &ReplicationManager{
		region: "eu-west-2",
	}
}

// createAWSCommand creates an OS-compatible AWS CLI command
func createAWSCommand(args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		// On Windows, use cmd.exe to properly handle the AWS CLI
		cmdArgs := append([]string{"/C", "aws"}, args...)
		return exec.Command("cmd", cmdArgs...)
	}
	// On Unix-like systems (Linux, macOS), execute directly
	return exec.Command("aws", args...)
}

// ValidEnvironments returns the list of valid environments
func (rm *ReplicationManager) ValidEnvironments() []string {
	return []string{"snd", "dev", "sit", "preprod", "trg", "prod", "qa", "stage"}
}

// Status retrieves the status of Blue-Green deployments for an environment
func (rm *ReplicationManager) Status(env string) (string, error) {
	if !rm.isValidEnv(env) {
		return "", fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(rm.ValidEnvironments(), ", "))
	}

	deployments, err := rm.listDeployments(env)
	if err != nil {
		return "", err
	}

	if len(deployments) == 0 {
		return fmt.Sprintf("No Blue-Green deployments found for environment: %s\n", env), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Blue-Green Deployments for %s:\n", env))
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	for _, d := range deployments {
		sb.WriteString(fmt.Sprintf("\nDeployment: %s\n", d.Name))
		sb.WriteString(fmt.Sprintf("  Identifier:  %s\n", d.Identifier))
		sb.WriteString(fmt.Sprintf("  Status:      %s\n", rm.formatStatus(d.Status)))
		if d.StatusDetails != "" {
			sb.WriteString(fmt.Sprintf("  Details:     %s\n", d.StatusDetails))
		}
		sb.WriteString(fmt.Sprintf("  Source:      %s\n", rm.extractClusterName(d.Source)))
		sb.WriteString(fmt.Sprintf("  Target:      %s\n", rm.extractClusterName(d.Target)))
		sb.WriteString(fmt.Sprintf("  Created:     %s\n", d.CreateTime.Format("2006-01-02 15:04:05")))

		if len(d.Tasks) > 0 {
			sb.WriteString("  Tasks:\n")
			for _, t := range d.Tasks {
				sb.WriteString(fmt.Sprintf("    - %s: %s\n", t.Name, t.Status))
			}
		}
	}

	return sb.String(), nil
}

// Switch performs a switchover of a Blue-Green deployment
func (rm *ReplicationManager) Switch(env, deploymentID string) error {
	if !rm.isValidEnv(env) {
		return fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(rm.ValidEnvironments(), ", "))
	}

	// Get deployment to verify it exists and is in correct state
	deployment, err := rm.getDeployment(deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment == nil {
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}

	// Verify deployment is in AVAILABLE state
	if deployment.Status != "AVAILABLE" {
		return fmt.Errorf("deployment is not ready for switchover (status: %s, required: AVAILABLE)", deployment.Status)
	}

	fmt.Printf("Starting switchover for deployment: %s\n", deployment.Name)
	fmt.Printf("  Source: %s\n", rm.extractClusterName(deployment.Source))
	fmt.Printf("  Target: %s\n", rm.extractClusterName(deployment.Target))
	fmt.Println()

	// Execute switchover
	cmd := createAWSCommand("rds", "switchover-blue-green-deployment",
		"--blue-green-deployment-identifier", deploymentID,
		"--region", rm.region,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("switchover failed: %s", stderr.String())
	}

	fmt.Println("✓ Switchover initiated successfully")
	fmt.Println("\nMonitoring progress...")

	// Monitor progress
	return rm.monitorSwitchover(deploymentID)
}

// monitorSwitchover monitors the switchover progress until completion
func (rm *ReplicationManager) monitorSwitchover(deploymentID string) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)
	lastStatus := ""

	for {
		select {
		case <-timeout:
			return fmt.Errorf("switchover timed out after 30 minutes")
		case <-ticker.C:
			deployment, err := rm.getDeployment(deploymentID)
			if err != nil {
				fmt.Printf("  ⚠ Error checking status: %v\n", err)
				continue
			}

			if deployment == nil {
				// Deployment may have been deleted after successful switchover
				fmt.Println("\n✓ Switchover completed - deployment cleaned up")
				return nil
			}

			if deployment.Status != lastStatus {
				lastStatus = deployment.Status
				fmt.Printf("  Status: %s\n", rm.formatStatus(deployment.Status))
			}

			switch deployment.Status {
			case "SWITCHOVER_COMPLETED":
				fmt.Println("\n✓ Switchover completed successfully!")
				return nil
			case "SWITCHOVER_FAILED":
				return fmt.Errorf("switchover failed: %s", deployment.StatusDetails)
			case "DELETING", "DELETED":
				fmt.Println("\n✓ Switchover completed - deployment being cleaned up")
				return nil
			}
		}
	}
}

// Create creates a new Blue-Green deployment
func (rm *ReplicationManager) Create(env, name, sourceCluster string) error {
	if !rm.isValidEnv(env) {
		return fmt.Errorf("invalid environment: %s (valid: %s)", env, strings.Join(rm.ValidEnvironments(), ", "))
	}

	if name == "" {
		return fmt.Errorf("deployment name is required")
	}

	if sourceCluster == "" {
		return fmt.Errorf("source cluster ARN or identifier is required")
	}

	// Build source ARN if not already an ARN
	sourceARN := sourceCluster
	if !strings.HasPrefix(sourceCluster, "arn:") {
		// Assume it's a cluster identifier, build the ARN
		sourceARN = fmt.Sprintf("arn:aws:rds:%s::cluster:%s", rm.region, sourceCluster)
	}

	fmt.Printf("Creating Blue-Green deployment:\n")
	fmt.Printf("  Name:   %s\n", name)
	fmt.Printf("  Source: %s\n", sourceCluster)
	fmt.Println()

	cmd := createAWSCommand("rds", "create-blue-green-deployment",
		"--blue-green-deployment-name", name,
		"--source", sourceARN,
		"--region", rm.region,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create deployment: %s", stderr.String())
	}

	// Parse response to get deployment ID
	var response struct {
		BlueGreenDeployment BlueGreenDeployment `json:"BlueGreenDeployment"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		fmt.Println("✓ Deployment creation initiated")
		return nil
	}

	fmt.Printf("✓ Deployment created successfully!\n")
	fmt.Printf("  Identifier: %s\n", response.BlueGreenDeployment.Identifier)
	fmt.Printf("  Status:     %s\n", rm.formatStatus(response.BlueGreenDeployment.Status))
	fmt.Println("\nUse 'rwcli replication status' to monitor progress")

	return nil
}

// Delete deletes a Blue-Green deployment
func (rm *ReplicationManager) Delete(deploymentID string, deleteTarget bool) error {
	if deploymentID == "" {
		return fmt.Errorf("deployment identifier is required")
	}

	// Verify deployment exists
	deployment, err := rm.getDeployment(deploymentID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment == nil {
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}

	fmt.Printf("Deleting Blue-Green deployment: %s\n", deployment.Name)

	args := []string{"rds", "delete-blue-green-deployment",
		"--blue-green-deployment-identifier", deploymentID,
		"--region", rm.region,
	}

	if deleteTarget {
		args = append(args, "--delete-target")
	}

	cmd := createAWSCommand(args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete deployment: %s", stderr.String())
	}

	fmt.Println("✓ Deployment deletion initiated")
	return nil
}

// listDeployments lists all Blue-Green deployments, optionally filtered by environment
func (rm *ReplicationManager) listDeployments(env string) ([]BlueGreenDeployment, error) {
	cmd := createAWSCommand("rds", "describe-blue-green-deployments",
		"--region", rm.region,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %s", stderr.String())
	}

	var response BlueGreenDeploymentsResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Filter by environment if specified
	if env != "" {
		filtered := make([]BlueGreenDeployment, 0)
		envLower := strings.ToLower(env)
		for _, d := range response.BlueGreenDeployments {
			// Check if deployment name or source contains the environment
			nameLower := strings.ToLower(d.Name)
			sourceLower := strings.ToLower(d.Source)
			if strings.Contains(nameLower, envLower) || strings.Contains(sourceLower, envLower) {
				filtered = append(filtered, d)
			}
		}
		return filtered, nil
	}

	return response.BlueGreenDeployments, nil
}

// getDeployment retrieves a specific deployment by ID
func (rm *ReplicationManager) getDeployment(deploymentID string) (*BlueGreenDeployment, error) {
	cmd := createAWSCommand("rds", "describe-blue-green-deployments",
		"--blue-green-deployment-identifier", deploymentID,
		"--region", rm.region,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check if it's a "not found" error
		if strings.Contains(stderr.String(), "BlueGreenDeploymentNotFoundFault") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get deployment: %s", stderr.String())
	}

	var response BlueGreenDeploymentsResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.BlueGreenDeployments) == 0 {
		return nil, nil
	}

	return &response.BlueGreenDeployments[0], nil
}

// formatStatus formats the status with emoji indicators
func (rm *ReplicationManager) formatStatus(status string) string {
	switch status {
	case "AVAILABLE":
		return "✓ AVAILABLE (ready for switchover)"
	case "PROVISIONING":
		return "⏳ PROVISIONING"
	case "SWITCHOVER_IN_PROGRESS":
		return "🔄 SWITCHOVER_IN_PROGRESS"
	case "SWITCHOVER_COMPLETED":
		return "✓ SWITCHOVER_COMPLETED"
	case "SWITCHOVER_FAILED":
		return "✗ SWITCHOVER_FAILED"
	case "DELETING":
		return "🗑 DELETING"
	case "DELETED":
		return "🗑 DELETED"
	default:
		return status
	}
}

// extractClusterName extracts the cluster name from an ARN
func (rm *ReplicationManager) extractClusterName(arn string) string {
	if arn == "" {
		return "(none)"
	}
	parts := strings.Split(arn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return arn
}

func (rm *ReplicationManager) isValidEnv(env string) bool {
	for _, e := range rm.ValidEnvironments() {
		if e == env {
			return true
		}
	}
	return false
}

// ConfirmReplicationSwitch prompts the user for confirmation before switchover
func ConfirmReplicationSwitch(deploymentName, source, target string) bool {
	fmt.Printf("\n⚠️  WARNING: You are about to perform a Blue-Green switchover!\n")
	fmt.Printf("   Deployment: %s\n", deploymentName)
	fmt.Printf("   Source:     %s\n", source)
	fmt.Printf("   Target:     %s\n", target)
	fmt.Printf("\n   This will switch production traffic to the target cluster.\n")
	fmt.Printf("   Type 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}

// ConfirmReplicationCreate prompts the user for confirmation before creating deployment
func ConfirmReplicationCreate(name, source string) bool {
	fmt.Printf("\n⚠️  Creating a new Blue-Green deployment:\n")
	fmt.Printf("   Name:   %s\n", name)
	fmt.Printf("   Source: %s\n", source)
	fmt.Printf("\n   This will create a clone of the source cluster.\n")
	fmt.Printf("   Type 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}

// ConfirmReplicationDelete prompts the user for confirmation before deleting deployment
func ConfirmReplicationDelete(deploymentName string, deleteTarget bool) bool {
	fmt.Printf("\n⚠️  WARNING: You are about to delete a Blue-Green deployment!\n")
	fmt.Printf("   Deployment: %s\n", deploymentName)
	if deleteTarget {
		fmt.Printf("   ⚠️  Target cluster will also be DELETED!\n")
	}
	fmt.Printf("\n   Type 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}
