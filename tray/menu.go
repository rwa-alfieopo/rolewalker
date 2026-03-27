package tray

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"rolewalkers/internal/db"

	"github.com/getlantern/systray"
)

// buildInitialMenu creates the menu structure once. Items are stored on the
// app struct so refreshMenu can update their titles dynamically.
func (a *app) buildInitialMenu() {
	// --- Header ---
	a.mStatus = systray.AddMenuItem("", "Current AWS profile")
	a.mStatus.Disable()

	a.mKube = systray.AddMenuItem("", "Kubernetes context")
	a.mKube.Disable()

	systray.AddSeparator()

	// --- Environments ---
	a.addEnvironmentItems()

	systray.AddSeparator()

	// --- Namespaces ---
	a.addKubeSection()

	systray.AddSeparator()

	// --- Quit ---
	mQuit := systray.AddMenuItem("Quit", "Quit rolewalkers tray")
	go func() {
		<-mQuit.ClickedCh
		close(a.quit)
		if a.database != nil {
			a.database.Close()
		}
		systray.Quit()
	}()

	// Set initial labels
	a.refreshLabels()
}

// addEnvironmentItems adds a menu item per environment from the database.
// Environments like DEV and TRG that share an account appear as separate items.
func (a *app) addEnvironmentItems() {
	if a.dbRepo == nil {
		mErr := systray.AddMenuItem("⚠ Database not available", "")
		mErr.Disable()
		return
	}

	envs, err := a.dbRepo.GetAllEnvironments()
	if err != nil {
		mErr := systray.AddMenuItem("⚠ Failed to load environments", err.Error())
		mErr.Disable()
		return
	}

	for _, e := range envs {
		env := e // capture
		item := systray.AddMenuItem("", fmt.Sprintf("Switch to %s (%s)", env.DisplayName, env.Name))
		a.envItems = append(a.envItems, envItem{item: item, env: env})

		go func() {
			for {
				<-item.ClickedCh
				a.switchEnvironment(env)
				a.refreshMenu()
			}
		}()
	}
}

// switchEnvironment handles switching to an environment from the tray.
// It switches the AWS profile (if needed) and then the kube context.
func (a *app) switchEnvironment(env db.Environment) {
	profileName := env.AWSProfile

	// Check if SSO login is needed for this profile
	needsLogin := a.sm != nil && !a.sm.IsLoggedIn(profileName)

	if needsLogin {
		fmt.Fprintf(os.Stderr, "SSO session expired for %s, logging in...\n", profileName)
		if err := a.sm.Login(profileName); err != nil {
			fmt.Fprintf(os.Stderr, "SSO login failed for %s: %v\n", profileName, err)
			return
		}
		fmt.Fprintf(os.Stderr, "SSO login successful for %s\n", profileName)
	}

	// Switch AWS profile
	if err := a.ps.SwitchProfile(profileName); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to switch profile to %s: %v\n", profileName, err)
		return
	}

	// Switch kube context to the environment's specific cluster
	if err := a.km.SwitchContextForEnv(env.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Kube context switch to %s failed: %v\n", env.ClusterName, err)
	}

	fmt.Fprintf(os.Stderr, "Switched to: %s (profile: %s, cluster: %s)\n",
		env.DisplayName, profileName, env.ClusterName)
}

// refreshMenu updates all dynamic labels. Safe to call from any goroutine.
func (a *app) refreshMenu() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refreshLabels()
}

// refreshLabels updates the tray title and all menu item labels.
func (a *app) refreshLabels() {
	active := a.cm.GetActiveProfile()
	region := a.ps.GetDefaultRegion()

	// Determine active environment name from kube context
	activeEnv := a.resolveActiveEnv()

	// Tray title — show environment name if we can resolve it
	title := fmt.Sprintf("☁ %s", activeEnv)
	if activeEnv == active {
		// Couldn't resolve to env name, show profile
		title = fmt.Sprintf("☁ %s", active)
	}
	if region != "" {
		title += fmt.Sprintf(" (%s)", region)
	}
	systray.SetTitle(title)

	// Status header
	a.mStatus.SetTitle(fmt.Sprintf("Active: %s  [%s]", activeEnv, active))

	// Kube context
	kubeCtx := "(none)"
	if ctx, err := a.km.GetCurrentContext(); err == nil && ctx != "" {
		if strings.Contains(ctx, "/") {
			parts := strings.Split(ctx, "/")
			ctx = parts[len(parts)-1]
		}
		kubeCtx = ctx
	}
	kubeNS := a.km.GetCurrentNamespace()
	if kubeNS == "" {
		kubeNS = "default"
	}
	a.mKube.SetTitle(fmt.Sprintf("⎈ %s / %s", kubeCtx, kubeNS))

	// Environment items
	for i := range a.envItems {
		ei := &a.envItems[i]
		isActive := (ei.env.Name == activeEnv)
		ei.item.SetTitle(a.formatEnvLabel(ei.env, isActive))
	}

	// Namespace items
	namespaces := []string{"zenith", "tunnel-access", "default", "kube-system"}
	for i, item := range a.nsItems {
		if i < len(namespaces) {
			label := "  " + namespaces[i]
			if namespaces[i] == kubeNS {
				label = "✓ " + namespaces[i]
			}
			item.SetTitle(label)
		}
	}
}

// resolveActiveEnv tries to determine which environment is active by matching
// the current kube context to an environment's cluster name.
func (a *app) resolveActiveEnv() string {
	ctx, err := a.km.GetCurrentContext()
	if err != nil || ctx == "" {
		return a.cm.GetActiveProfile()
	}

	for _, ei := range a.envItems {
		if strings.Contains(ctx, ei.env.ClusterName) {
			return ei.env.Name
		}
	}

	// Fallback: try to extract from context name (e.g. "dev-zenith-eks-cluster" -> "dev")
	if strings.Contains(ctx, "/") {
		parts := strings.Split(ctx, "/")
		ctx = parts[len(parts)-1]
	}
	for _, ei := range a.envItems {
		if strings.HasPrefix(ctx, ei.env.Name+"-") {
			return ei.env.Name
		}
	}

	return a.cm.GetActiveProfile()
}

// formatEnvLabel builds the display label for an environment menu item.
func (a *app) formatEnvLabel(env db.Environment, isActive bool) string {
	label := fmt.Sprintf("%s (%s)", env.DisplayName, env.Name)
	if isActive {
		label = "✓ " + label
	} else {
		label = "  " + label
	}

	// SSO status — check the profile this environment uses
	if a.sm != nil {
		if a.sm.IsLoggedIn(env.AWSProfile) {
			remaining := a.getSessionTimeLeft(env.AWSProfile)
			if remaining != "" {
				label += fmt.Sprintf("  [%s]", remaining)
			} else {
				label += "  [SSO ✓]"
			}
		} else {
			label += "  [SSO ✗]"
		}
	}

	return label
}

// getSessionTimeLeft returns a human-readable string of time remaining.
func (a *app) getSessionTimeLeft(profileName string) string {
	if a.sm == nil {
		return ""
	}

	expiry, err := a.sm.GetCredentialExpiry(profileName)
	if err != nil || expiry == nil {
		return ""
	}

	remaining := time.Until(*expiry)
	if remaining <= 0 {
		return "expired"
	}

	hours := int(math.Floor(remaining.Hours()))
	minutes := int(math.Floor(remaining.Minutes())) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm left", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm left", minutes)
	}
	return "< 1m left"
}

// addKubeSection adds namespace quick-switch items.
func (a *app) addKubeSection() {
	mNSHeader := systray.AddMenuItem("Namespaces", "")
	mNSHeader.Disable()

	namespaces := []string{"zenith", "tunnel-access", "default", "kube-system"}

	for _, ns := range namespaces {
		namespace := ns
		item := systray.AddMenuItem("", fmt.Sprintf("Switch to namespace %s", namespace))
		a.nsItems = append(a.nsItems, item)

		go func() {
			for {
				<-item.ClickedCh
				if err := a.km.SetNamespace(namespace); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to set namespace %s: %v\n", namespace, err)
				} else {
					fmt.Fprintf(os.Stderr, "Namespace set to: %s\n", namespace)
					a.refreshMenu()
				}
			}
		}()
	}
}
