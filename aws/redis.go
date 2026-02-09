package aws

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"rolewalkers/internal/k8s"
	"rolewalkers/internal/utils"
	"strings"
)

// RedisManager handles Redis connection operations
type RedisManager struct {
	kubeManager     *KubeManager
	ssmManager      *SSMManager
	profileSwitcher *ProfileSwitcher
}

// NewRedisManager creates a new RedisManager instance
func NewRedisManager() *RedisManager {
	ps, _ := NewProfileSwitcher()
	return &RedisManager{
		kubeManager:     NewKubeManager(),
		ssmManager:      NewSSMManager(),
		profileSwitcher: ps,
	}
}

// Connect spawns an interactive redis-cli pod to connect to the Redis cluster
func (rm *RedisManager) Connect(env string) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := rm.kubeManager.SwitchContextForEnvWithProfile(env, rm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get Redis endpoint from SSM
	fmt.Println("Fetching Redis endpoint...")
	endpointPath := fmt.Sprintf("/%s/zenith/redis/cluster-endpoint", env)
	endpoint, err := rm.ssmManager.GetParameter(endpointPath)
	if err != nil {
		return fmt.Errorf("failed to get Redis endpoint: %w", err)
	}

	// Get Redis password from SSM
	fmt.Println("Fetching Redis credentials...")
	passwordPath := fmt.Sprintf("/%s/zenith/redis/zenithmaster-password", env)
	password, err := rm.ssmManager.GetParameter(passwordPath)
	if err != nil {
		return fmt.Errorf("failed to get Redis password: %w", err)
	}

	// Parse endpoint to extract host (remove port if present)
	host := parseRedisHost(endpoint)

	// Generate unique pod name
	username := utils.GetCurrentUsernamePodSafe()
	if username == "unknown" {
		username = "user"
	}
	podName := fmt.Sprintf("redis-temp-%s-%d", username, rand.IntN(10000))

	fmt.Printf("\nConnecting to Redis:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Host:        %s\n", host)
	fmt.Printf("  Port:        6379\n")
	fmt.Printf("  User:        zenithmaster\n")
	fmt.Printf("  Pod:         %s\n", podName)
	fmt.Println()

	return rm.runRedisPodWithStatus(podName, host, password)
}

// parseRedisHost extracts the host from an endpoint (removes port if present)
func parseRedisHost(endpoint string) string {
	// Remove any trailing port (e.g., "redis.example.com:6379" -> "redis.example.com")
	if idx := strings.LastIndex(endpoint, ":"); idx != -1 {
		// Check if what follows is a port number
		port := endpoint[idx+1:]
		if _, err := fmt.Sscanf(port, "%d", new(int)); err == nil {
			return endpoint[:idx]
		}
	}
	return endpoint
}

// runRedisPodWithStatus spawns an interactive redis-cli pod and shows status updates
func (rm *RedisManager) runRedisPodWithStatus(podName, host, password string) error {
	// Build labels with creator identity
	labels := k8s.CreatorLabelsWithSession()

	fmt.Println("Creating Redis CLI pod...")
	fmt.Println("Status: Pulling image redis:7-alpine...")
	
	cmd := exec.Command("kubectl", "run", podName,
		"--rm", "-it",
		"--restart=Never",
		"--namespace=tunnel-access",
		"--image=redis:7-alpine",
		"--labels", labels,
		"--env", fmt.Sprintf("REDISCLI_AUTH=%s", password),
		"--",
		"redis-cli",
		"-h", host,
		"-p", "6379",
		"-c",           // Enable cluster mode
		"--tls",        // Use TLS (required for AWS ElastiCache)
		"--insecure",   // Skip TLS cert verification
		"--user", "zenithmaster",
	)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Status: Starting container...")
	fmt.Println("Status: Connecting to Redis cluster...")
	fmt.Println("\nStarting interactive redis-cli session (cluster mode)...")
	fmt.Println("(Type 'quit' or Ctrl+D to exit)")
	fmt.Println()

	err := cmd.Run()
	if err != nil {
		// Check if it's just the user exiting normally
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				return nil
			}
		}
	}

	return err
}
