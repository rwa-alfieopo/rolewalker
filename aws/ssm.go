package aws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"rolewalkers/internal/awscli"
	"strings"
)

// SSMManager handles AWS SSM parameter operations
type SSMManager struct {
	region string
}

// NewSSMManager creates a new SSM manager
func NewSSMManager() *SSMManager {
	return &SSMManager{
		region: "eu-west-2",
	}
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
		return "", fmt.Errorf("failed to get SSM parameter %s: %s", name, stderr.String())
	}

	var resp ssmResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("failed to parse SSM response: %w", err)
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

	switch service {
	case "db", "database":
		// Default to query/read endpoint
		return fmt.Sprintf("/%s/zenith/database/query/db-read-endpoint", env)
	case "db-write":
		return fmt.Sprintf("/%s/zenith/database/query/db-write-endpoint", env)
	case "db-command":
		return fmt.Sprintf("/%s/zenith/database/command/db-write-endpoint", env)
	case "db-command-read":
		return fmt.Sprintf("/%s/zenith/database/command/db-read-endpoint", env)
	case "redis":
		return fmt.Sprintf("/%s/zenith/redis/cluster-endpoint", env)
	case "elasticsearch", "es":
		return fmt.Sprintf("/%s/zenith/elasticsearch/cluster-endpoint", env)
	case "kafka":
		return fmt.Sprintf("/%s/zenith/kafka/broker", env)
	case "msk":
		return fmt.Sprintf("/%s/zenith/msk/brokers-iam-endpoint", env)
	case "rabbitmq":
		return fmt.Sprintf("/%s/zenith/rabbitmq/brokers-console-url", env)
	default:
		return ""
	}
}

// GetDatabaseEndpoint retrieves database endpoint with node type (read/write) and db type (query/command)
func (sm *SSMManager) GetDatabaseEndpoint(env, nodeType, dbType string) (string, error) {
	nodeType = strings.ToLower(nodeType)
	dbType = strings.ToLower(dbType)

	if nodeType != "read" && nodeType != "write" {
		nodeType = "read"
	}
	if dbType != "query" && dbType != "command" {
		dbType = "query"
	}

	paramPath := fmt.Sprintf("/%s/zenith/database/%s/db-%s-endpoint", env, dbType, nodeType)
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
		return nil, fmt.Errorf("failed to list SSM parameters at %s: %s", prefix, stderr.String())
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
