package aws

import (
	"fmt"
	"rolewalkers/internal/k8s"
	"strconv"
	"strings"
)

// RedisManager handles Redis connection operations
type RedisManager struct {
	kubeManager     *KubeManager
	ssmManager      *SSMManager
	profileSwitcher *ProfileSwitcher
}

// NewRedisManagerWithDeps creates a new RedisManager with shared dependencies
func NewRedisManagerWithDeps(km *KubeManager, ssm *SSMManager, ps *ProfileSwitcher) *RedisManager {
	return &RedisManager{
		kubeManager:     km,
		ssmManager:      ssm,
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

	fmt.Printf("\nConnecting to Redis:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Host:        %s\n", host)
	fmt.Printf("  Port:        6379\n")
	fmt.Printf("  User:        zenithmaster\n")
	fmt.Println()

	return rm.runRedisPod(host, password)
}

// parseRedisHost extracts the host from an endpoint (removes port if present)
func parseRedisHost(endpoint string) string {
	// Remove any trailing port (e.g., "redis.example.com:6379" -> "redis.example.com")
	if idx := strings.LastIndex(endpoint, ":"); idx != -1 {
		// Check if what follows is a port number
		port := endpoint[idx+1:]
		if _, err := strconv.Atoi(port); err == nil {
			return endpoint[:idx]
		}
	}
	return endpoint
}

// runRedisPod spawns an interactive redis-cli pod
func (rm *RedisManager) runRedisPod(host, password string) error {
	fmt.Println("Creating Redis CLI pod...")
	fmt.Println("Status: Pulling image redis:7-alpine...")
	fmt.Println("Status: Starting container...")
	fmt.Println("Status: Connecting to Redis cluster...")
	fmt.Println("\nStarting interactive redis-cli session (cluster mode)...")
	fmt.Println("(Type 'quit' or Ctrl+D to exit)")
	fmt.Println()

	return k8s.RunPod(k8s.PodSpec{
		NamePrefix:  "redis-temp",
		Image:       "redis:7-alpine",
		Interactive: true,
		Command:     []string{"redis-cli", "-h", host, "-p", "6379", "-c", "--tls", "--user", "zenithmaster"},
		Env:         map[string]string{"REDISCLI_AUTH": password},
	})
}
