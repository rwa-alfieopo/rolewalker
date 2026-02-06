package k8s

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PodManager provides common Kubernetes pod operations
type PodManager struct {
	namespace string
}

// NewPodManager creates a new PodManager for the specified namespace
func NewPodManager(namespace string) *PodManager {
	if namespace == "" {
		namespace = "default"
	}
	return &PodManager{namespace: namespace}
}

// PodExists checks if a pod exists in the namespace
func (pm *PodManager) PodExists(podName string) bool {
	cmd := exec.Command("kubectl", "get", "pod", podName, "-n", pm.namespace, "-o", "name")
	return cmd.Run() == nil
}

// DeletePod deletes a pod from the namespace
func (pm *PodManager) DeletePod(podName string) error {
	cmd := exec.Command("kubectl", "delete", "pod", podName, "-n", pm.namespace, "--grace-period=0", "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}

	return nil
}

// GetPodStatus returns the current status phase of a pod
func (pm *PodManager) GetPodStatus(podName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pod", podName,
		"-n", pm.namespace,
		"-o", "jsonpath={.status.phase}",
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// WaitForPodReady waits for a pod to be in Running state with timeout
func (pm *PodManager) WaitForPodReady(podName string, timeout time.Duration) error {
	timeoutChan := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for pod to be ready")
		case <-ticker.C:
			status, err := pm.GetPodStatus(podName)
			if err != nil {
				continue
			}

			switch status {
			case "Running":
				fmt.Println("✓ Pod is running")
				return nil
			case "Failed", "Error", "CrashLoopBackOff":
				return fmt.Errorf("pod entered %s state", status)
			default:
				fmt.Printf("  Pod status: %s\n", status)
			}
		}
	}
}

// WaitForPodReadyKubectl waits for a pod using kubectl wait command
func (pm *PodManager) WaitForPodReadyKubectl(podName string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "-n", pm.namespace, "wait", "pods",
		"-l", fmt.Sprintf("name=%s", podName),
		"--for", "condition=Ready",
		"--timeout", fmt.Sprintf("%.0fs", timeout.Seconds()),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, stderr.String())
	}

	return nil
}
