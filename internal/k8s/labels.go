package k8s

import (
	"fmt"
	"os"
	"rolewalkers/internal/utils"
	"strings"
	"time"
)

// labelPairs builds the common creator labels and appends any extra key=value pairs.
func labelPairs(extras ...string) string {
	username := utils.GetCurrentUsername()
	email := utils.GetCurrentEmail()

	base := []string{
		"created-by=" + username,
		"creator-email=" + email,
	}
	return strings.Join(append(base, extras...), ",")
}

// CreatorLabels generates standard Kubernetes labels for pod creator identity.
func CreatorLabels() string {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	return labelPairs("created-at=" + timestamp)
}

// CreatorLabelsWithSession generates labels including a session ID for temporary pods.
func CreatorLabelsWithSession() string {
	sessionID := fmt.Sprintf("%d", os.Getpid())
	return labelPairs("session-id=" + sessionID)
}

// CreatorLabelsWithOperation generates labels including an operation type.
func CreatorLabelsWithOperation(operation string) string {
	sessionID := fmt.Sprintf("%d", os.Getpid())
	return labelPairs("session-id="+sessionID, "operation="+operation)
}

// CreatorLabelsWithName generates labels including a pod name.
func CreatorLabelsWithName(podName string) string {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	return labelPairs("name="+podName, "created-at="+timestamp)
}
