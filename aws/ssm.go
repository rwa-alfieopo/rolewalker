package aws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"rolewalkers/internal/awscli"
	"rolewalkers/internal/config"
	"rolewalkers/internal/db"
	"strings"
)

// SSMManager handles AWS SSM parameter operations
type SSMManager struct {
	region     string
	configRepo *db.ConfigRepository
}

// NewSSMManager creates a new SSM manager
func NewSSMManager() *SSMManager {
	cfg := config.Get()
	return &SSMManager{region: cfg.Region, configRepo: nil}
}

// NewSSMManagerWithRepo creates a new SSM manager with a shared config repository
func NewSSMManagerWithRepo(repo *db.ConfigRepository) *SSMManager {
	cfg := config.Get()
	return &SSMManager{region: cfg.Region, configRepo: repo}
}

// ssmResponse represents the AWS SSM get-parameter response
type ssmResponse struct {
	Parameter struct {
		Value string `json:"Value"`
	} `json:"Parameter"`
}

// GetParameter retrieves a parameter from SSM Parameter Store
func (sm *SSMManager) GetParameter(name string) (string, error) {
	cmd := awscli.CreateCommand("ssm", "get-parameter",
		"--name", name,
		"--with-decryption",
		"--region", sm.region,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get SSM parameter %s: %w: %s", name, err, stderr.String())
	}

	var resp ssmResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("failed to parse SSM response: %w", err)
	}

	if resp.Parameter.Value == "" {
		return "", fmt.Errorf("SSM parameter %s exists but has empty value", name)
	}

	return resp.Parameter.Value, nil
}

// GetEndpoint retrieves a service endpoint from SSM for a given environment
func (sm *SSMManager) GetEndpoint(env, service string) (string, error) {
	// Map service names to SSM parameter paths
	paramPath := sm.getParameterPath(env, service)
	if paramPath == "" {
		return "", fmt.Errorf("unknown service: %s", service)
	}

	return sm.GetParameter(paramPath)
}

// getParameterPath returns the SSM parameter path for a service
func (sm *SSMManager) getParameterPath(env, service string) string {
	service = strings.ToLower(service)
	env = strings.ToLower(env)
	cfg := config.Get()

	switch service {
	case "db", "database":
		return cfg.SSMPath(env, "database/query/db-read-endpoint")
	case "db-write":
		return cfg.SSMPath(env, "database/query/db-write-endpoint")
	case "db-command":
		return cfg.SSMPath(env, "database/command/db-write-endpoint")
	case "db-command-read":
		return cfg.SSMPath(env, "database/command/db-read-endpoint")
	case "redis":
		return cfg.SSMPath(env, "redis/cluster-endpoint")
	case "elasticsearch", "es":
		return cfg.SSMPath(env, "elasticsearch/cluster-endpoint")
	case "kafka":
		return cfg.SSMPath(env, "kafka/broker")
	case "msk":
		return cfg.SSMPath(env, "msk/brokers-iam-endpoint")
	case "rabbitmq":
		return cfg.SSMPath(env, "rabbitmq/brokers-console-url")
	default:
		return ""
	}
}

// GetDatabaseEndpoint retrieves database endpoint with node type (read/write) and db type (query/command)
func (sm *SSMManager) GetDatabaseEndpoint(env, nodeType, dbType string) (string, error) {
	cfg := config.Get()
	nodeType = strings.ToLower(nodeType)
	dbType = strings.ToLower(dbType)

	if nodeType != "read" && nodeType != "write" {
		nodeType = "read"
	}
	if dbType != "query" && dbType != "command" {
		dbType = "query"
	}

	paramPath := cfg.SSMPath(env, fmt.Sprintf("database/%s/db-%s-endpoint", dbType, nodeType))
	return sm.GetParameter(paramPath)
}

// ssmListResponse represents the AWS SSM get-parameters-by-path response
type ssmListResponse struct {
	Parameters []struct {
		Name string `json:"Name"`
		Type string `json:"Type"`
	} `json:"Parameters"`
}

// ListParameters lists all parameters under a given path prefix
func (sm *SSMManager) ListParameters(prefix string) ([]string, error) {
	cmd := awscli.CreateCommand("ssm", "get-parameters-by-path",
		"--path", prefix,
		"--recursive",
		"--region", sm.region,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to list SSM parameters at %s: %w: %s", prefix, err, stderr.String())
	}

	var resp ssmListResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse SSM response: %w", err)
	}

	names := make([]string, len(resp.Parameters))
	for i, p := range resp.Parameters {
		names[i] = p.Name
	}

	return names, nil
}
