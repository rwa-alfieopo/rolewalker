package aws

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// MSKManager handles MSK Kafka UI operations
type MSKManager struct {
	kubeManager *KubeManager
	ssmManager  *SSMManager
}

// NewMSKManager creates a new MSKManager instance
func NewMSKManager() *MSKManager {
	return &MSKManager{
		kubeManager: NewKubeManager(),
		ssmManager:  NewSSMManager(),
	}
}

// StartUI deploys a Kafka UI pod and port-forwards to localhost
func (mm *MSKManager) StartUI(env string, localPort int) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := mm.kubeManager.SwitchContextForEnv(env); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get MSK brokers from SSM
	fmt.Println("Fetching MSK brokers endpoint...")
	brokersPath := fmt.Sprintf("/%s/zenith/msk/brokers-iam-endpoint", env)
	brokers, err := mm.ssmManager.GetParameter(brokersPath)
	if err != nil {
		return fmt.Errorf("failed to get MSK brokers: %w", err)
	}

	podName := fmt.Sprintf("kafka-ui-%s", env)

	// Check if pod already exists
	if mm.podExists(podName) {
		fmt.Printf("Pod %s already exists, reusing...\n", podName)
	} else {
		// Create the Kafka UI pod
		fmt.Printf("Creating Kafka UI pod: %s\n", podName)
		if err := mm.createKafkaUIPod(podName, env, brokers); err != nil {
			return fmt.Errorf("failed to create Kafka UI pod: %w", err)
		}

		// Wait for pod to be ready
		fmt.Println("Waiting for pod to be ready...")
		if err := mm.waitForPod(podName); err != nil {
			// Cleanup on failure
			mm.deletePod(podName)
			return fmt.Errorf("pod failed to start: %w", err)
		}
	}

	fmt.Printf("\nStarting Kafka UI port-forward:\n")
	fmt.Printf("  Pod:       %s\n", podName)
	fmt.Printf("  Namespace: default\n")
	fmt.Printf("  Local:     http://localhost:%d\n", localPort)
	fmt.Printf("  Brokers:   %s\n", truncateBrokers(brokers))
	fmt.Println("\nPress Ctrl+C to stop (pod will remain running)...")
	fmt.Printf("To stop the pod later: rwcli msk stop %s\n\n", env)

	return mm.startPortForward(podName, localPort)
}

// StopUI deletes the Kafka UI pod for an environment
func (mm *MSKManager) StopUI(env string) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := mm.kubeManager.SwitchContextForEnv(env); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	podName := fmt.Sprintf("kafka-ui-%s", env)

	if !mm.podExists(podName) {
		return fmt.Errorf("pod %s not found in namespace default", podName)
	}

	fmt.Printf("Deleting Kafka UI pod: %s\n", podName)
	if err := mm.deletePod(podName); err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	fmt.Printf("✓ Kafka UI pod stopped: %s\n", podName)
	return nil
}

// createKafkaUIPod creates the Kafka UI pod with IAM authentication
func (mm *MSKManager) createKafkaUIPod(podName, env, brokers string) error {
	cmd := exec.Command("kubectl", "run", podName,
		"--restart=Never",
		"--image=provectuslabs/kafka-ui:latest",
		fmt.Sprintf("--env=KAFKA_CLUSTERS_0_NAME=%s", env),
		fmt.Sprintf("--env=KAFKA_CLUSTERS_0_BOOTSTRAPSERVERS=%s", brokers),
		"--env=KAFKA_CLUSTERS_0_PROPERTIES_SECURITY_PROTOCOL=SASL_SSL",
		"--env=KAFKA_CLUSTERS_0_PROPERTIES_SASL_MECHANISM=AWS_MSK_IAM",
		"--env=KAFKA_CLUSTERS_0_PROPERTIES_SASL_JAAS_CONFIG=software.amazon.msk.auth.iam.IAMLoginModule required;",
		"--env=KAFKA_CLUSTERS_0_PROPERTIES_SASL_CLIENT_CALLBACK_HANDLER_CLASS=software.amazon.msk.auth.iam.IAMClientCallbackHandler",
		"-n", "default",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}

	return nil
}

// waitForPod waits for the pod to be in Running state
func (mm *MSKManager) waitForPod(podName string) error {
	timeout := time.After(120 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for pod to be ready")
		case <-ticker.C:
			status, err := mm.getPodStatus(podName)
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

// getPodStatus returns the current status of a pod
func (mm *MSKManager) getPodStatus(podName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pod", podName,
		"-n", "default",
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

// podExists checks if a pod exists in the default namespace
func (mm *MSKManager) podExists(podName string) bool {
	cmd := exec.Command("kubectl", "get", "pod", podName, "-n", "default", "-o", "name")
	return cmd.Run() == nil
}

// deletePod deletes a pod from the default namespace
func (mm *MSKManager) deletePod(podName string) error {
	cmd := exec.Command("kubectl", "delete", "pod", podName, "-n", "default", "--grace-period=0", "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}

	return nil
}

// startPortForward runs kubectl port-forward with interrupt handling
func (mm *MSKManager) startPortForward(podName string, localPort int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n\nStopping port-forward...")
		cancel()
	}()

	cmd := exec.CommandContext(ctx, "kubectl", "port-forward",
		fmt.Sprintf("pod/%s", podName),
		fmt.Sprintf("%d:8080", localPort),
		"-n", "default",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	if ctx.Err() == context.Canceled {
		fmt.Println("✓ Port-forward stopped")
		fmt.Printf("  Pod %s is still running. Use 'rwcli msk stop %s' to delete it.\n", podName, strings.TrimPrefix(podName, "kafka-ui-"))
		return nil
	}

	return err
}

// truncateBrokers shortens the brokers string for display
func truncateBrokers(brokers string) string {
	if len(brokers) > 60 {
		return brokers[:57] + "..."
	}
	return brokers
}
