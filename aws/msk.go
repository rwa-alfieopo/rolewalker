package aws

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"rolewalkers/internal/k8s"
	"rolewalkers/internal/utils"
	"strings"
	"syscall"
	"time"
)

// MSKManager handles MSK Kafka UI operations
type MSKManager struct {
	kubeManager     *KubeManager
	ssmManager      *SSMManager
	profileSwitcher *ProfileSwitcher
}

// NewMSKManager creates a new MSKManager instance
func NewMSKManager() *MSKManager {
	ps, err := NewProfileSwitcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Profile switcher init failed: %v\n", err)
	}
	return &MSKManager{
		kubeManager:     NewKubeManager(),
		ssmManager:      NewSSMManager(),
		profileSwitcher: ps,
	}
}

// StartUI deploys a Kafka UI pod and port-forwards to localhost
func (mm *MSKManager) StartUI(env string, localPort int) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := mm.kubeManager.SwitchContextForEnvWithProfile(env, mm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get MSK brokers from SSM
	fmt.Println("Fetching MSK brokers endpoint...")
	brokersPath := fmt.Sprintf("/%s/zenith/msk/brokers-iam-endpoint", env)
	brokers, err := mm.ssmManager.GetParameter(brokersPath)
	if err != nil {
		return fmt.Errorf("failed to get MSK brokers: %w", err)
	}

	// Get username for pod name
	username := utils.GetCurrentUsername()
	if username == "unknown" {
		username = "user"
	}

	podName := fmt.Sprintf("kafka-ui-%s-%s", env, username)

	// Check if pod already exists
	podMgr := k8s.NewPodManager("default")
	if podMgr.PodExists(podName) {
		fmt.Printf("Pod %s already exists, reusing...\n", podName)
	} else {
		// Create the Kafka UI pod
		fmt.Printf("Creating Kafka UI pod: %s\n", podName)
		if err := mm.createKafkaUIPod(podName, env, brokers); err != nil {
			return fmt.Errorf("failed to create Kafka UI pod: %w", err)
		}

		// Wait for pod to be ready
		fmt.Println("Waiting for pod to be ready...")
		if err := podMgr.WaitForPodReady(podName, 120*time.Second); err != nil {
			// Cleanup on failure
			podMgr.DeletePod(podName)
			return fmt.Errorf("pod failed to start: %w", err)
		}
	}

	fmt.Printf("\nStarting Kafka UI port-forward:\n")
	fmt.Printf("  Pod:       %s\n", podName)
	fmt.Printf("  Namespace: default\n")
	fmt.Printf("  Local:     http://localhost:%d\n", localPort)
	fmt.Printf("  Brokers:   %s\n", utils.TruncateString(brokers, 60))
	fmt.Printf("\nPress Ctrl+C to stop (pod will remain running)...")
	fmt.Printf("To stop the pod later: rw msk stop %s\n\n", env)

	return mm.startPortForward(podName, localPort)
}

// StopUI deletes the Kafka UI pod for an environment
func (mm *MSKManager) StopUI(env string) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := mm.kubeManager.SwitchContextForEnvWithProfile(env, mm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	// Get username for pod name
	username := utils.GetCurrentUsername()
	if username == "unknown" {
		username = "user"
	}

	podName := fmt.Sprintf("kafka-ui-%s-%s", env, username)

	podMgr := k8s.NewPodManager("default")
	if !podMgr.PodExists(podName) {
		return fmt.Errorf("pod %s not found in namespace default", podName)
	}

	fmt.Printf("Deleting Kafka UI pod: %s\n", podName)
	if err := podMgr.DeletePod(podName); err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	fmt.Printf("✓ Kafka UI pod stopped: %s\n", podName)
	return nil
}

// createKafkaUIPod creates the Kafka UI pod with IAM authentication
func (mm *MSKManager) createKafkaUIPod(podName, env, brokers string) error {
	// Build labels with creator identity
	labels := k8s.CreatorLabels()

	cmd := exec.Command("kubectl", "run", podName,
		"--restart=Never",
		"--image=provectuslabs/kafka-ui:latest",
		"--labels", labels,
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
		return fmt.Errorf("kubectl error: %s", stderr.String())
	}

	return nil
}

// startPortForward runs kubectl port-forward with interrupt handling
func (mm *MSKManager) startPortForward(podName string, localPort int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling with buffered channel to prevent goroutine leak
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan) // Cleanup signal notification

	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n\nStopping port-forward...")
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
		}
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
		fmt.Printf("  Pod %s is still running. Use 'rw msk stop %s' to delete it.\n", podName, strings.TrimPrefix(podName, "kafka-ui-"))
		return nil
	}

	return err
}
