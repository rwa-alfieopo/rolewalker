package k8s

import (
	"fmt"
	"os"
	"rolewalkers/internal/utils"
	"time"
)

// CreatorLabels generates standard Kubernetes labels for pod creator identity
func CreatorLabels() string {
	username := utils.GetCurrentUsername()
	email := utils.GetCurrentEmail()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	return fmt.Sprintf("created-by=%s,created-at=%s,creator-email=%s",
		username, timestamp, email)
}

// CreatorLabelsWithSession generates labels including a session ID for temporary pods
func CreatorLabelsWithSession() string {
	username := utils.GetCurrentUsername()
	email := utils.GetCurrentEmail()
	sessionID := fmt.Sprintf("%d", os.Getpid()) // Use PID for temp pods

	return fmt.Sprintf("created-by=%s,creator-email=%s,session-id=%s",
		username, email, sessionID)
}

// CreatorLabelsWithOperation generates labels including an operation type
func CreatorLabelsWithOperation(operation string) string {
	username := utils.GetCurrentUsername()
	email := utils.GetCurrentEmail()
	sessionID := fmt.Sprintf("%d", os.Getpid())

	return fmt.Sprintf("created-by=%s,creator-email=%s,session-id=%s,operation=%s",
		username, email, sessionID, operation)
}

// CreatorLabelsWithName generates labels including a pod name
func CreatorLabelsWithName(podName string) string {
	username := utils.GetCurrentUsername()
	email := utils.GetCurrentEmail()
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	return fmt.Sprintf("name=%s,created-by=%s,created-at=%s,creator-email=%s",
		podName, username, timestamp, email)
}
