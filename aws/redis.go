package aws

import (
	"fmt"
	"rolewalkers/internal/config"
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
	cfg := config.Get()

	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := rm.kubeManager.SwitchContextForEnvWithProfile(env, rm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	fmt.Println("Fetching Redis endpoint...")
	endpointPath := cfg.SSMPath(env, "redis/cluster-endpoint")
	endpoint, err := rm.ssmManager.GetParameter(endpointPath)
	if err != nil {
		return fmt.Errorf("failed to get Redis endpoint: %w", err)
	}

	fmt.Println("Fetching Redis credentials...")
	passwordPath := cfg.SSMPath(env, fmt.Sprintf("redis/%s-password", cfg.Database.RedisUser))
	password, err := rm.ssmManager.GetParameter(passwordPath)
	if err != nil {
		return fmt.Errorf("failed to get Redis password: %w", err)
	}

	host := parseRedisHost(endpoint)

	fmt.Printf("\nConnecting to Redis:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Host:        %s\n", host)
	fmt.Printf("  Port:        %d\n", cfg.Database.RedisPort)
	fmt.Printf("  User:        %s\n", cfg.Database.RedisUser)
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
	cfg := config.Get()
	fmt.Println("Starting interactive redis-cli session (cluster mode)...")
	fmt.Println("(Type 'quit' or Ctrl+D to exit)")
	fmt.Println()

	port := fmt.Sprintf("%d", cfg.Database.RedisPort)
	return k8s.RunPod(k8s.PodSpec{
		NamePrefix:  "redis-temp",
		Image:       cfg.Images.Redis,
		Interactive: true,
		Command:     []string{"redis-cli", "-h", host, "-p", port, "-c", "--tls", "--user", cfg.Database.RedisUser},
		Env:         map[string]string{"REDISCLI_AUTH": password},
	})
}
