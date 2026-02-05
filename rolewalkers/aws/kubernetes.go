package aws

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// KubeManager handles Kubernetes context operations
type KubeManager struct{}

// KubeContext represents a kubectl context
type KubeContext struct {
	Name      string
	Cluster   string
	IsCurrent bool
}

// NewKubeManager creates a new KubeManager instance
func NewKubeManager() *KubeManager {
	return &KubeManager{}
}

// GetContexts returns all available kubectl contexts
func (km *KubeManager) GetContexts() ([]KubeContext, error) {
	cmd := exec.Command("kubectl", "config", "get-contexts", "--no-headers")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get kubectl contexts: %s", stderr.String())
	}

	var contexts []KubeContext
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Format: CURRENT   NAME   CLUSTER   AUTHINFO   NAMESPACE
		// Current context has * in first column
		isCurrent := strings.HasPrefix(line, "*")
		fields := strings.Fields(line)

		if len(fields) < 2 {
			continue
		}

		var name, cluster string
		if isCurrent {
			// Skip the * marker
			if len(fields) >= 2 {
				name = fields[1]
			}
			if len(fields) >= 3 {
				cluster = fields[2]
			}
		} else {
			name = fields[0]
			if len(fields) >= 2 {
				cluster = fields[1]
			}
		}

		contexts = append(contexts, KubeContext{
			Name:      name,
			Cluster:   cluster,
			IsCurrent: isCurrent,
		})
	}

	return contexts, nil
}

// GetCurrentContext returns the current kubectl context name
func (km *KubeManager) GetCurrentContext() (string, error) {
	cmd := exec.Command("kubectl", "config", "current-context")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get current context: %s", stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// SwitchContext switches to the specified kubectl context
func (km *KubeManager) SwitchContext(contextName string) error {
	cmd := exec.Command("kubectl", "config", "use-context", contextName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to switch context: %s", stderr.String())
	}

	return nil
}

// FindContextForEnv finds a matching kubectl context for the given environment
// Supports patterns:
// - ARN format: arn:aws:eks:eu-west-2:{account}:cluster/{env}-zenith-eks-cluster
// - Simple format: zenith-{env}
func (km *KubeManager) FindContextForEnv(env string) (string, error) {
	contexts, err := km.GetContexts()
	if err != nil {
		return "", err
	}

	// Extract env name from profile (e.g., "zenith-dev" -> "dev")
	envName := extractEnvName(env)

	// Pattern matchers
	arnPattern := regexp.MustCompile(fmt.Sprintf(`arn:aws:eks:[^:]+:\d+:cluster/%s-zenith-eks-cluster`, regexp.QuoteMeta(envName)))
	simplePattern := fmt.Sprintf("zenith-%s", envName)

	for _, ctx := range contexts {
		// Check ARN format
		if arnPattern.MatchString(ctx.Name) {
			return ctx.Name, nil
		}
		// Check simple format
		if ctx.Name == simplePattern {
			return ctx.Name, nil
		}
		// Check cluster name for ARN pattern
		if arnPattern.MatchString(ctx.Cluster) {
			return ctx.Name, nil
		}
	}

	return "", fmt.Errorf("no matching kubectl context found for environment '%s'", envName)
}

// SwitchContextForEnv finds and switches to the kubectl context for the given environment
func (km *KubeManager) SwitchContextForEnv(env string) error {
	contextName, err := km.FindContextForEnv(env)
	if err != nil {
		return err
	}

	return km.SwitchContext(contextName)
}

// extractEnvName extracts the environment name from a profile name
// e.g., "zenith-dev" -> "dev", "zenith-prod" -> "prod", "dev" -> "dev"
func extractEnvName(profileName string) string {
	// Remove common prefixes
	name := strings.TrimPrefix(profileName, "zenith-")
	name = strings.TrimPrefix(name, "aws-")

	// Handle cases like "zenith-dev-admin" -> "dev"
	parts := strings.Split(name, "-")
	if len(parts) > 0 {
		return parts[0]
	}

	return name
}

// ListContextsFormatted returns a formatted string of all contexts
func (km *KubeManager) ListContextsFormatted() (string, error) {
	contexts, err := km.GetContexts()
	if err != nil {
		return "", err
	}

	if len(contexts) == 0 {
		return "No kubectl contexts found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Kubernetes Contexts:\n")
	sb.WriteString(strings.Repeat("-", 80) + "\n")

	for _, ctx := range contexts {
		marker := "  "
		if ctx.IsCurrent {
			marker = "* "
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", marker, ctx.Name))
		if ctx.Cluster != "" && ctx.Cluster != ctx.Name {
			sb.WriteString(fmt.Sprintf("    Cluster: %s\n", ctx.Cluster))
		}
	}

	return sb.String(), nil
}
