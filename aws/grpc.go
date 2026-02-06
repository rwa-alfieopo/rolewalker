package aws

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"rolewalkers/internal/db"
	"sort"
	"strings"
	"syscall"
)

// GRPCManager handles gRPC port-forwarding operations
type GRPCManager struct {
	kubeManager     *KubeManager
	profileSwitcher *ProfileSwitcher
	configRepo      *db.ConfigRepository
}

// NewGRPCManager creates a new GRPCManager instance
func NewGRPCManager() *GRPCManager {
	ps, _ := NewProfileSwitcher()
	database, err := db.NewDB()
	var repo *db.ConfigRepository
	if err == nil {
		repo = db.NewConfigRepository(database)
	}
	return &GRPCManager{
		kubeManager:     NewKubeManager(),
		profileSwitcher: ps,
		configRepo:      repo,
	}
}

// GetServicePort returns the local port for a gRPC service
func (gm *GRPCManager) GetServicePort(service string) (int, error) {
	service = strings.ToLower(service)
	
	if gm.configRepo != nil {
		microservices, err := gm.configRepo.GetGRPCMicroservices()
		if err == nil {
			if port, ok := microservices[service]; ok {
				return port, nil
			}
		}
	}
	
	return 0, fmt.Errorf("unknown gRPC service: %s\nAvailable: %s", service, gm.GetServices())
}

// GetServices returns a comma-separated list of available gRPC services
func (gm *GRPCManager) GetServices() string {
	if gm.configRepo != nil {
		microservices, err := gm.configRepo.GetGRPCMicroservices()
		if err == nil {
			services := make([]string, 0, len(microservices))
			for s := range microservices {
				services = append(services, s)
			}
			sort.Strings(services)
			return strings.Join(services, ", ")
		}
	}
	return "candidate, job, client, organisation, user, email, billing, core"
}

// GetServiceName returns the Kubernetes service name for a gRPC microservice
func (gm *GRPCManager) GetServiceName(service string) string {
	return fmt.Sprintf("%s-microservice-grpc", strings.ToLower(service))
}

// ListServices returns a formatted list of all gRPC services and their ports
func (gm *GRPCManager) ListServices() string {
	var sb strings.Builder
	sb.WriteString("gRPC Services:\n")
	sb.WriteString(strings.Repeat("-", 50) + "\n")
	sb.WriteString(fmt.Sprintf("%-15s %-10s %s\n", "SERVICE", "PORT", "K8S SERVICE"))
	sb.WriteString(strings.Repeat("-", 50) + "\n")

	if gm.configRepo != nil {
		microservices, err := gm.configRepo.GetGRPCMicroservices()
		if err == nil {
			services := make([]string, 0, len(microservices))
			for s := range microservices {
				services = append(services, s)
			}
			sort.Strings(services)

			for _, service := range services {
				port := microservices[service]
				k8sService := gm.GetServiceName(service)
				sb.WriteString(fmt.Sprintf("%-15s %-10d %s\n", service, port, k8sService))
			}

			sb.WriteString("\nUsage: rw grpc <service> <env>\n")
			sb.WriteString("Example: rw grpc candidate dev\n")
			return sb.String()
		}
	}

	sb.WriteString("Database not available. Please initialize the database.\n")
	return sb.String()
}

// Forward starts port-forwarding to a gRPC service
func (gm *GRPCManager) Forward(service, env string) error {
	service = strings.ToLower(service)
	env = strings.ToLower(env)

	// Validate service
	localPort, err := gm.GetServicePort(service)
	if err != nil {
		return err
	}

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := gm.kubeManager.SwitchContextForEnvWithProfile(env, gm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	k8sService := gm.GetServiceName(service)
	remotePort := localPort // gRPC services use the same port locally and remotely

	fmt.Printf("\nStarting gRPC port-forward:\n")
	fmt.Printf("  Service:   %s\n", k8sService)
	fmt.Printf("  Namespace: zenith\n")
	fmt.Printf("  Local:     localhost:%d\n", localPort)
	fmt.Printf("  Remote:    %d\n", remotePort)
	fmt.Println("\nPress Ctrl+C to stop...")

	return gm.startPortForward(k8sService, localPort, remotePort)
}

// startPortForward runs kubectl port-forward with interrupt handling
func (gm *GRPCManager) startPortForward(serviceName string, localPort, remotePort int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling with buffered channel to prevent goroutine leak
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan) // Cleanup signal notification

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n\nStopping port-forward...")
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
		}
	}()

	cmd := exec.CommandContext(ctx, "kubectl", "port-forward",
		fmt.Sprintf("svc/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"-n", "zenith",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if ctx.Err() == context.Canceled {
		fmt.Println("✓ Port-forward stopped")
		return nil
	}

	return err
}

// CheckServiceExists verifies if a gRPC service exists in the cluster
func (gm *GRPCManager) CheckServiceExists(service, env string) error {
	k8sService := gm.GetServiceName(service)

	cmd := exec.Command("kubectl", "get", "svc", k8sService, "-n", "zenith", "-o", "name")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("service %s not found in namespace zenith: %s", k8sService, stderr.String())
	}

	return nil
}
