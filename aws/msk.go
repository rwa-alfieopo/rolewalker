package aws

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"rolewalkers/internal/config"
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

// NewMSKManagerWithDeps creates a new MSKManager with shared dependencies
func NewMSKManagerWithDeps(km *KubeManager, ssm *SSMManager, ps *ProfileSwitcher) *MSKManager {
	return &MSKManager{
		kubeManager:     km,
		ssmManager:      ssm,
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
	cfg := config.Get()
	brokersPath := cfg.SSMPath(env, "msk/brokers-iam-endpoint")
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
	cfg := config.Get()
	labels := k8s.CreatorLabels()

	cmd := exec.Command("kubectl", "run", podName,
		"--restart=Never",
		fmt.Sprintf("--image=%s", cfg.Images.KafkaUI),
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

// ConnectCLI spawns an interactive Kafka CLI pod with IAM authentication
func (mm *MSKManager) ConnectCLI(env string) error {
	env = strings.ToLower(env)

	// Switch kubectl context to the environment
	fmt.Printf("Switching kubectl context to %s...\n", env)
	if err := mm.kubeManager.SwitchContextForEnvWithProfile(env, mm.profileSwitcher); err != nil {
		return fmt.Errorf("failed to switch kubectl context: %w", err)
	}

	fmt.Println("Fetching MSK brokers endpoint...")
	cfg := config.Get()
	brokersPath := cfg.SSMPath(env, "msk/brokers-iam-endpoint")
	brokers, err := mm.ssmManager.GetParameter(brokersPath)
	if err != nil {
		return fmt.Errorf("failed to get MSK brokers: %w", err)
	}

	fmt.Printf("\nStarting Kafka CLI session:\n")
	fmt.Printf("  Environment: %s\n", env)
	fmt.Printf("  Brokers:     %s\n", utils.TruncateString(brokers, 60))
	fmt.Printf("  Auth:        IAM (SASL_SSL)\n")
	fmt.Println("\nUseful commands inside the pod:")
	fmt.Println("  kafka-topics --bootstrap-server $BOOTSTRAP_SERVERS --command-config /tmp/client.properties --list")
	fmt.Println("  kafka-console-consumer --bootstrap-server $BOOTSTRAP_SERVERS --consumer.config /tmp/client.properties --topic <topic>")
	fmt.Println()

	// Build the init command that downloads the IAM auth JAR and creates client.properties
	initScript := fmt.Sprintf(`
set -e
BOOTSTRAP_SERVERS="%s"
export BOOTSTRAP_SERVERS

# Download AWS MSK IAM auth library
IAM_JAR_URL="https://github.com/aws/aws-msk-iam-auth/releases/download/v2.3.4/aws-msk-iam-auth-2.3.4-all.jar"
echo "Downloading MSK IAM auth library..."
wget -q -O /tmp/aws-msk-iam-auth.jar "$IAM_JAR_URL" 2>/dev/null || \
  curl -sL -o /tmp/aws-msk-iam-auth.jar "$IAM_JAR_URL"

# Create client.properties for IAM auth
cat > /tmp/client.properties << 'EOF'
security.protocol=SASL_SSL
sasl.mechanism=AWS_MSK_IAM
sasl.jaas.config=software.amazon.msk.auth.iam.IAMLoginModule required;
sasl.client.callback.handler.class=software.amazon.msk.auth.iam.IAMClientCallbackHandler
EOF

export CLASSPATH="/tmp/aws-msk-iam-auth.jar"
echo "Ready. BOOTSTRAP_SERVERS=$BOOTSTRAP_SERVERS"
echo "Use --command-config /tmp/client.properties with kafka-* commands"
exec /bin/bash
`, brokers)

	return k8s.RunPod(k8s.PodSpec{
		NamePrefix:  "msk-cli",
		Image:       cfg.Images.KafkaCLI,
		Namespace:   TunnelAccessNamespace(),
		Interactive: true,
		Command:     []string{"/bin/bash", "-c", initScript},
		Env: map[string]string{
			"BOOTSTRAP_SERVERS": brokers,
		},
	})
}
