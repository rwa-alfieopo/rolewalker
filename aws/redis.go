package aws

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// RedisManager handles Redis connection operations
type RedisManager struct {
	kubeManager *KubeManager
	ssmManager  *SSMManager
}

// NewRedisManager creates a new RedisManager instance
func NewRedisManager() *RedisManager {
	return &RedisManager{
		kubeManager: NewKubeManager(),
		ssmManager:  NewSSMManager(),
	}
}

// Connect spawns an interactive redis-cli pod to connect to the Redis cluster
func (rm *RedisManager) Connect(env string) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := rm.kubeManager.SwitchContextForEnv(env); err != nil {
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
	username := sanitizeRedisUsername(os.Getenv("USER"))
	if username == "" {
		username = sanitizeRedisUsername(os.Getenv("USERNAME"))
	}
	if username == "" {
		username = "user"
	}
	podName := fmt.Sprintf("redis-temp-%s-%d", username, rand.Intn(10000))

	fmt.Printf("\nConnecting to Redis:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Host:        %s\n", host)
	fmt.Printf("  Port:        6379\n")
	fmt.Printf("  User:        zenithmaster\n")
	fmt.Printf("  Pod:         %s\n", podName)
	fmt.Println("\nStarting interactive redis-cli session (cluster mode)...")
	fmt.Println("(Type 'quit' or Ctrl+D to exit)\n")

	return rm.runRedisPod(podName, host, password)
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

// runRedisPod spawns an interactive redis-cli pod
func (rm *RedisManager) runRedisPod(podName, host, password string) error {
	cmd := exec.Command("kubectl", "run", podName,
		"--rm", "-it",
		"--restart=Never",
		"--namespace=tunnel-access",
		"--image=redis:7-alpine",
		"--",
		"redis-cli",
		"-h", host,
		"-p", "6379",
		"-c",           // Enable cluster mode
		"--tls",        // Use TLS (required for AWS ElastiCache)
		"--insecure",   // Skip TLS cert verification
		"--user", "zenithmaster",
		"--pass", password,
	)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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

// sanitizeRedisUsername removes non-alphanumeric characters and lowercases username
func sanitizeRedisUsername(username string) string {
	username = strings.ToLower(username)
	re := regexp.MustCompile(`[^a-z0-9-]`)
	result := re.ReplaceAllString(username, "")
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}
