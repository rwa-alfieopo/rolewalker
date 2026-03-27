package tray

import (
	"fmt"
	"os"
	"strings"

	"rolewalkers/aws"

	"github.com/getlantern/systray"
)

// buildMenu rebuilds the entire tray menu. Called on init and refresh.
// systray doesn't support removing items, so we build once and update
// the tray title on subsequent calls.
func (a *app) buildMenu() {
	a.mu.Lock()
	defer a.mu.Unlock()

	active := a.cm.GetActiveProfile()
	region := a.ps.GetDefaultRegion()

	title := fmt.Sprintf("☁ %s", active)
	if region != "" {
		title += fmt.Sprintf(" (%s)", region)
	}
	systray.SetTitle(title)
}

// buildInitialMenu is called once during onReady to create the menu structure.
// We use a goroutine-per-item pattern for click handling.
func (a *app) buildInitialMenu() {
	active := a.cm.GetActiveProfile()
	region := a.ps.GetDefaultRegion()

	// --- Header: current context ---
	title := fmt.Sprintf("☁ %s", active)
	if region != "" {
		title += fmt.Sprintf(" (%s)", region)
	}
	systray.SetTitle(title)

	mStatus := systray.AddMenuItem(fmt.Sprintf("Active: %s", active), "Current AWS profile")
	mStatus.Disable()

	// Kube context
	kubeCtx := "(none)"
	if ctx, err := a.km.GetCurrentContext(); err == nil && ctx != "" {
		// Shorten ARN-style names
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
	mKube := systray.AddMenuItem(fmt.Sprintf("⎈ %s / %s", kubeCtx, kubeNS), "Kubernetes context")
	mKube.Disable()

	systray.AddSeparator()

	// --- Profiles section ---
	profiles, err := a.cm.GetProfiles()
	if err != nil {
		mErr := systray.AddMenuItem("⚠ Failed to load profiles", err.Error())
		mErr.Disable()
	} else {
		a.addProfileItems(profiles, active)
	}

	systray.AddSeparator()

	// --- Kubernetes namespaces ---
	a.addKubeSection()

	systray.AddSeparator()

	// --- Quit ---
	mQuit := systray.AddMenuItem("Quit", "Quit rolewalkers tray")
	go func() {
		<-mQuit.ClickedCh
		close(a.quit)
		if a.db != nil {
			a.db.Close()
		}
		systray.Quit()
	}()
}

// addProfileItems adds a menu item per profile with SSO status.
func (a *app) addProfileItems(profiles []aws.Profile, active string) {
	for _, p := range profiles {
		profile := p // capture for goroutine

		label := profile.Name
		if profile.IsActive {
			label = "✓ " + label
		} else {
			label = "  " + label
		}

		if profile.IsSSO && a.sm != nil {
			if a.sm.IsLoggedIn(profile.Name) {
				label += "  (SSO ✓)"
			} else {
				label += "  (SSO ✗)"
			}
		}

		if profile.Region != "" {
			label += "  " + profile.Region
		}

		item := systray.AddMenuItem(label, fmt.Sprintf("Switch to %s", profile.Name))

		go func() {
			for {
				<-item.ClickedCh
				a.switchProfile(profile)
			}
		}()
	}
}

// switchProfile handles switching to a profile from the tray.
func (a *app) switchProfile(profile aws.Profile) {
	// If SSO and not logged in, login first
	if profile.IsSSO && a.sm != nil && !a.sm.IsLoggedIn(profile.Name) {
		fmt.Fprintf(os.Stderr, "Logging in to %s...\n", profile.Name)
		if err := a.sm.Login(profile.Name); err != nil {
			fmt.Fprintf(os.Stderr, "SSO login failed for %s: %v\n", profile.Name, err)
			return
		}
	}

	if err := a.ps.SwitchProfile(profile.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to switch to %s: %v\n", profile.Name, err)
		return
	}

	// Switch kube context
	if err := a.km.SwitchContextForEnv(profile.Name); err != nil {
		// Non-fatal — profile switch succeeded
		fmt.Fprintf(os.Stderr, "Kube context switch failed: %v\n", err)
	}

	// Update tray title
	title := fmt.Sprintf("☁ %s", profile.Name)
	if profile.Region != "" {
		title += fmt.Sprintf(" (%s)", profile.Region)
	}
	systray.SetTitle(title)

	fmt.Fprintf(os.Stderr, "Switched to: %s\n", profile.Name)
}

// addKubeSection adds namespace quick-switch items for common namespaces.
func (a *app) addKubeSection() {
	mKubeHeader := systray.AddMenuItem("Namespaces", "")
	mKubeHeader.Disable()

	namespaces := []string{"zenith", "tunnel-access", "default", "kube-system"}

	for _, ns := range namespaces {
		namespace := ns // capture
		currentNS := a.km.GetCurrentNamespace()

		label := "  " + namespace
		if namespace == currentNS {
			label = "✓ " + namespace
		}

		item := systray.AddMenuItem(label, fmt.Sprintf("Switch to namespace %s", namespace))

		go func() {
			for {
				<-item.ClickedCh
				if err := a.km.SetNamespace(namespace); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to set namespace %s: %v\n", namespace, err)
				} else {
					fmt.Fprintf(os.Stderr, "Namespace set to: %s\n", namespace)
				}
			}
		}()
	}
}
