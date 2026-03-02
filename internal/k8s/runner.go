package k8s

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"rolewalkers/internal/utils"
	"strings"
)

// PodSpec describes a temporary Kubernetes pod to run via kubectl.
type PodSpec struct {
	// Prefix for the generated pod name (e.g. "psql", "redis-temp", "dbtunnel").
	NamePrefix string

	// Container image (e.g. "postgres:15-alpine", "redis:7-alpine").
	Image string

	// Namespace to run in. Defaults to "tunnel-access" if empty.
	Namespace string

	// Command to run inside the container (e.g. ["psql", "-h", "host"]).
	Command []string

	// Environment variables as name→value pairs. Passed via pod spec
	// overrides so they don't appear in the process list.
	Env map[string]string

	// Interactive means the pod needs stdin/tty attached (--rm -it).
	// Non-interactive pods use --rm -i (for piped I/O).
	Interactive bool

	// Labels operation type (e.g. "backup", "restore"). Empty uses session labels.
	Operation string

	// Stdin overrides os.Stdin when set (e.g. for piping a file).
	Stdin io.Reader

	// Stdout overrides os.Stdout when set (e.g. for capturing to a file).
	Stdout io.Writer

	// Stderr overrides os.Stderr when set.
	Stderr io.Writer
}

// PodResult holds the output from a non-interactive pod run.
type PodResult struct {
	Stdout string
	Stderr string
}

// GeneratePodName creates a unique pod name from the prefix and current user.
func GeneratePodName(prefix string) string {
	username := utils.GetCurrentUsernamePodSafe()
	if username == "unknown" {
		username = "user"
	}
	return fmt.Sprintf("%s-%s-%d", prefix, username, rand.IntN(10000))
}

// RunPod executes a temporary pod via kubectl run. It builds the appropriate
// overrides JSON to pass env vars securely and handles interactive vs piped I/O.
// Returns nil on success or normal user exit (exit code 0).
func RunPod(spec PodSpec) error {
	if spec.Namespace == "" {
		spec.Namespace = "tunnel-access"
	}

	podName := GeneratePodName(spec.NamePrefix)

	// Build labels
	var labels string
	if spec.Operation != "" {
		labels = CreatorLabelsWithOperation(spec.Operation)
	} else {
		labels = CreatorLabelsWithSession()
	}

	// Build overrides JSON
	overrides := buildOverrides(podName, spec)

	// Build kubectl args
	args := []string{"run", podName, "--rm"}
	if spec.Interactive {
		args = append(args, "-it")
	} else {
		args = append(args, "-i")
	}
	args = append(args,
		"--restart=Never",
		"--namespace="+spec.Namespace,
		"--image="+spec.Image,
		"--labels", labels,
		"--overrides", overrides,
		"--override-type=strategic",
	)

	cmd := exec.Command("kubectl", args...)

	// Wire I/O
	if spec.Stdin != nil {
		cmd.Stdin = spec.Stdin
	} else if spec.Interactive {
		cmd.Stdin = os.Stdin
	}
	if spec.Stdout != nil {
		cmd.Stdout = spec.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if spec.Stderr != nil {
		cmd.Stderr = spec.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 0 {
			return nil
		}
	}
	return err
}

// buildOverrides creates the JSON pod spec override string.
func buildOverrides(podName string, spec PodSpec) string {
	container := map[string]interface{}{
		"name":  podName,
		"image": spec.Image,
		"stdin": true,
	}

	if spec.Interactive {
		container["tty"] = true
	}

	if len(spec.Command) > 0 {
		container["command"] = spec.Command
	}

	if len(spec.Env) > 0 {
		var envVars []map[string]string
		// Sort keys for deterministic output
		for _, k := range sortedKeys(spec.Env) {
			envVars = append(envVars, map[string]string{
				"name":  k,
				"value": spec.Env[k],
			})
		}
		container["env"] = envVars
	}

	override := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []interface{}{container},
		},
	}

	data, _ := json.Marshal(override)
	return string(data)
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — maps are small
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && strings.Compare(keys[j-1], keys[j]) > 0; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
