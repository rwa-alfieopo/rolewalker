package aws

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"rolewalkers/internal/k8s"
	"rolewalkers/internal/utils"
	"strings"
	"syscall"
	"time"
)

// TunnelManager handles tunnel operations
type TunnelManager struct {
	kubeManager     *KubeManager
	ssmManager      *SSMManager
	portConfig      *PortConfig
	state           *TunnelState
	profileSwitcher *ProfileSwitcher
}

// TunnelConfig holds configuration for a tunnel
type TunnelConfig struct {
	Service     string
	Environment string
	NodeType    string // for db: read/write
	DBType      string // for db: query/command
}

// ServicePorts defines the remote port for each service
var ServicePorts = map[string]int{
	"db":            5432,
	"redis":         6379,
	"elasticsearch": 9200,
	"kafka":         9092,
	"msk":           9098,
	"rabbitmq":      443,
	"grpc":          5001,
}

// NewTunnelManager creates a new tunnel manager
func NewTunnelManager() (*TunnelManager, error) {
	state, err := NewTunnelState()
	if err != nil {
		return nil, err
	}

	ps, _ := NewProfileSwitcher()

	return &TunnelManager{
		kubeManager:     NewKubeManager(),
		ssmManager:      NewSSMManager(),
		portConfig:      NewPortConfig(),
		state:           state,
		profileSwitcher: ps,
	}, nil
}

// Start creates and starts a tunnel
func (tm *TunnelManager) Start(config TunnelConfig) error {
	service := strings.ToLower(config.Service)
	env := strings.ToLower(config.Environment)

	// Check if tunnel already exists
	tunnelID := GenerateTunnelID(service, env)
	if existing := tm.state.GetByServiceEnv(service, env); existing != nil {
		return fmt.Errorf("tunnel already exists: %s (pod: %s, port: %d)\nUse 'rwcli tunnel stop %s %s' to stop it first",
			tunnelID, existing.PodName, existing.LocalPort, service, env)
	}

	// Switch kubectl context to the environment
	if err := tm.kubeManager.SwitchContextForEnvWithProfile(env, tm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get remote endpoint from SSM
	remoteHost, err := tm.getRemoteHost(service, env, config)
	if err != nil {
		return fmt.Errorf("failed to get remote endpoint: %w", err)
	}

	// Get local port from port config
	localPorts, err := tm.portConfig.GetPort(service, env)
	if err != nil {
		return fmt.Errorf("failed to get local port: %w", err)
	}
	localPort := localPorts[0] // Use first port

	// Get remote port
	remotePort := ServicePorts[service]
	if remotePort == 0 {
		remotePort = 5432 // default
	}

	// Generate pod name
	username := utils.SanitizeUsername(os.Getenv("USER"))
	if username == "" {
		username = utils.SanitizeUsername(os.Getenv("USERNAME"))
	}
	if username == "" {
		username = "user"
	}
	podName := fmt.Sprintf("%stunnel-%s-%d", service, username, rand.Intn(10000))

	fmt.Printf("Creating tunnel: %s\n", tunnelID)
	fmt.Printf("  Pod: %s\n", podName)
	fmt.Printf("  Local: localhost:%d\n", localPort)
	fmt.Printf("  Remote: %s:%d\n", remoteHost, remotePort)

	// Create the socat pod
	if err := tm.createSocatPod(podName, remoteHost, remotePort); err != nil {
		return fmt.Errorf("failed to create tunnel pod: %w", err)
	}

	// Wait for pod to be ready
	fmt.Println("Waiting for pod to be ready...")
	if err := tm.waitForPod(podName); err != nil {
		tm.deletePod(podName)
		return fmt.Errorf("pod failed to start: %w", err)
	}

	// Save tunnel state
	tunnel := &TunnelInfo{
		ID:          tunnelID,
		Service:     service,
		Environment: env,
		PodName:     podName,
		LocalPort:   localPort,
		RemoteHost:  remoteHost,
		RemotePort:  remotePort,
		StartedAt:   time.Now(),
	}

	if err := tm.state.Add(tunnel); err != nil {
		tm.deletePod(podName)
		return fmt.Errorf("failed to save tunnel state: %w", err)
	}

	fmt.Printf("\n✓ Tunnel created successfully!\n")
	fmt.Printf("  Connect to: localhost:%d\n", localPort)
	fmt.Println("\nStarting port-forward (press Ctrl+C to stop)...")

	// Start port-forward with interrupt handling
	return tm.startPortForward(tunnel)
}

// getRemoteHost retrieves the remote host for a service
func (tm *TunnelManager) getRemoteHost(service, env string, config TunnelConfig) (string, error) {
	switch service {
	case "db":
		nodeType := config.NodeType
		dbType := config.DBType
		if nodeType == "" {
			nodeType = "read"
		}
		if dbType == "" {
			dbType = "query"
		}
		return tm.ssmManager.GetDatabaseEndpoint(env, nodeType, dbType)
	case "grpc":
		// gRPC uses direct service forwarding, not SSM
		return "", nil
	default:
		return tm.ssmManager.GetEndpoint(env, service)
	}
}

// createSocatPod creates a socat pod for tunneling
func (tm *TunnelManager) createSocatPod(podName, remoteHost string, remotePort int) error {
	// Build labels with creator identity
	labels := k8s.CreatorLabelsWithName(podName)

	cmd := exec.Command("kubectl", "-n", "tunnel-access", "run", podName,
		"--port", fmt.Sprintf("%d", remotePort),
		"--image", "alpine/socat",
		"--image-pull-policy", "IfNotPresent",
		"--labels", labels,
		"--command", "--",
		"socat", fmt.Sprintf("tcp-listen:%d,fork,reuseaddr", remotePort),
		fmt.Sprintf("tcp:%s:%d", remoteHost, remotePort),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, stderr.String())
	}

	return nil
}

// waitForPod waits for a pod to be ready
func (tm *TunnelManager) waitForPod(podName string) error {
	cmd := exec.Command("kubectl", "-n", "tunnel-access", "wait", "pods",
		"-l", fmt.Sprintf("name=%s", podName),
		"--for", "condition=Ready",
		"--timeout", "90s",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, stderr.String())
	}

	return nil
}

// startPortForward starts kubectl port-forward with interrupt handling
func (tm *TunnelManager) startPortForward(tunnel *TunnelInfo) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling with buffered channel to prevent goroutine leak
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan) // Cleanup signal notification

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n\nInterrupted, cleaning up tunnel...")
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
		}
	}()

	cmd := exec.CommandContext(ctx, "kubectl", "-n", "tunnel-access", "port-forward",
		fmt.Sprintf("pod/%s", tunnel.PodName),
		fmt.Sprintf("%d:%d", tunnel.LocalPort, tunnel.RemotePort),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	// Cleanup on exit
	tm.cleanup(tunnel)

	if ctx.Err() == context.Canceled {
		return nil // Normal interrupt
	}

	return err
}

// cleanup removes the tunnel pod and state
func (tm *TunnelManager) cleanup(tunnel *TunnelInfo) {
	fmt.Printf("Cleaning up tunnel: %s\n", tunnel.ID)
	tm.deletePod(tunnel.PodName)
	tm.state.Remove(tunnel.ID)
}

// deletePod deletes a kubernetes pod
func (tm *TunnelManager) deletePod(podName string) error {
	cmd := exec.Command("kubectl", "-n", "tunnel-access", "delete", "pod", podName)
	return cmd.Run()
}

// Stop stops a specific tunnel
func (tm *TunnelManager) Stop(service, env string) error {
	service = strings.ToLower(service)
	env = strings.ToLower(env)

	tunnel := tm.state.GetByServiceEnv(service, env)
	if tunnel == nil {
		return fmt.Errorf("no active tunnel found for %s-%s", service, env)
	}

	fmt.Printf("Stopping tunnel: %s\n", tunnel.ID)

	// Delete the pod
	if err := tm.deletePod(tunnel.PodName); err != nil {
		fmt.Printf("Warning: failed to delete pod %s: %v\n", tunnel.PodName, err)
	}

	// Remove from state
	if err := tm.state.Remove(tunnel.ID); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	fmt.Printf("✓ Tunnel stopped: %s\n", tunnel.ID)
	return nil
}

// StopAll stops all active tunnels
func (tm *TunnelManager) StopAll() error {
	tunnels := tm.state.List()
	if len(tunnels) == 0 {
		fmt.Println("No active tunnels to stop.")
		return nil
	}

	fmt.Printf("Stopping %d tunnel(s)...\n", len(tunnels))

	for _, tunnel := range tunnels {
		fmt.Printf("  Stopping %s...\n", tunnel.ID)
		if err := tm.deletePod(tunnel.PodName); err != nil {
			fmt.Printf("    Warning: failed to delete pod %s: %v\n", tunnel.PodName, err)
		}
	}

	// Clear all state
	if err := tm.state.Clear(); err != nil {
		return fmt.Errorf("failed to clear state: %w", err)
	}

	fmt.Println("✓ All tunnels stopped")
	return nil
}

// List returns formatted list of active tunnels
func (tm *TunnelManager) List() string {
	tunnels := tm.state.List()
	if len(tunnels) == 0 {
		return "No active tunnels.\n\nStart a tunnel with: rwcli tunnel start <service> <env>"
	}

	var sb strings.Builder
	sb.WriteString("Active Tunnels:\n")
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	for _, t := range tunnels {
		status := tm.checkPodStatus(t.PodName)
		sb.WriteString(fmt.Sprintf("\n%s:\n", t.ID))
		sb.WriteString(fmt.Sprintf("  Pod:     %s (%s)\n", t.PodName, status))
		sb.WriteString(fmt.Sprintf("  Local:   localhost:%d\n", t.LocalPort))
		sb.WriteString(fmt.Sprintf("  Remote:  %s:%d\n", t.RemoteHost, t.RemotePort))
		sb.WriteString(fmt.Sprintf("  Started: %s\n", t.StartedAt.Format("2006-01-02 15:04:05")))
	}

	return sb.String()
}

// checkPodStatus checks if a pod is running
func (tm *TunnelManager) checkPodStatus(podName string) string {
	cmd := exec.Command("kubectl", "-n", "tunnel-access", "get", "pod", podName,
		"-o", "jsonpath={.status.phase}")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "unknown"
	}

	return strings.TrimSpace(out.String())
}

// CleanupStale removes tunnels whose pods no longer exist
func (tm *TunnelManager) CleanupStale() error {
	tunnels := tm.state.List()
	cleaned := 0

	for _, tunnel := range tunnels {
		status := tm.checkPodStatus(tunnel.PodName)
		if status == "unknown" || status == "" {
			fmt.Printf("Removing stale tunnel: %s (pod not found)\n", tunnel.ID)
			tm.state.Remove(tunnel.ID)
			cleaned++
		}
	}

	if cleaned > 0 {
		fmt.Printf("✓ Cleaned up %d stale tunnel(s)\n", cleaned)
	} else {
		fmt.Println("No stale tunnels found.")
	}

	return nil
}

// GetSupportedServices returns list of supported tunnel services
func GetSupportedServices() string {
	return "db, redis, elasticsearch, kafka, msk, rabbitmq, grpc"
}
