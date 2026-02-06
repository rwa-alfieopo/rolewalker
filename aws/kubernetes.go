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
// Uses the exact cluster name mapping from AWS profiles
func (km *KubeManager) FindContextForEnv(env string) (string, error) {
	contexts, err := km.GetContexts()
	if err != nil {
		return "", err
	}

	// Map profile names to EKS cluster names
	clusterMap := map[string]string{
		"zenith-qa":      "qa-zenith-eks-cluster",
		"zenith-dev":     "dev-zenith-eks-cluster",
		"zenith-live":    "prod-zenith-eks-cluster",
		"zenith-sandbox": "snd-zenith-eks-cluster",
		"zenith-staging": "stage-zenith-eks-cluster",
	}

	// Get the cluster name for this profile
	clusterName, ok := clusterMap[env]
	if !ok {
		// Try extracting env name and building cluster name
		envName := extractEnvName(env)
		// Map common env names to cluster prefixes
		envToPrefix := map[string]string{
			"qa":      "qa",
			"dev":     "dev",
			"live":    "prod",
			"prod":    "prod",
			"sandbox": "snd",
			"snd":     "snd",
			"staging": "stage",
			"stage":   "stage",
			"preprod": "preprod",
			"sit":     "sit",
			"trg":     "trg",
		}
		prefix, found := envToPrefix[envName]
		if found {
			clusterName = prefix + "-zenith-eks-cluster"
		} else {
			clusterName = envName + "-zenith-eks-cluster"
		}
	}

	// Pattern to match ARN format contexts
	arnPattern := regexp.MustCompile(fmt.Sprintf(`arn:aws:eks:[^:]+:\d+:cluster/%s`, regexp.QuoteMeta(clusterName)))

	for _, ctx := range contexts {
		// Check if context name matches ARN pattern with cluster name
		if arnPattern.MatchString(ctx.Name) {
			return ctx.Name, nil
		}
		// Check if context name is the cluster name directly
		if ctx.Name == clusterName {
			return ctx.Name, nil
		}
		// Check cluster field for ARN pattern
		if arnPattern.MatchString(ctx.Cluster) {
			return ctx.Name, nil
		}
	}

	return "", fmt.Errorf("no matching kubectl context found for '%s' (looking for cluster: %s)", env, clusterName)
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
