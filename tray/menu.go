package tray

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"rolewalkers/aws"

	"github.com/getlantern/systray"
)

// buildInitialMenu creates the menu structure once. Items are stored on the
// app struct so refreshMenu can update their titles dynamically.
func (a *app) buildInitialMenu() {
	active := a.cm.GetActiveProfile()

	// --- Header ---
	a.mStatus = systray.AddMenuItem("", "Current AWS profile")
	a.mStatus.Disable()

	a.mKube = systray.AddMenuItem("", "Kubernetes context")
	a.mKube.Disable()

	systray.AddSeparator()

	// --- Profiles ---
	profiles, err := a.cm.GetProfiles()
	if err != nil {
		mErr := systray.AddMenuItem("⚠ Failed to load profiles", err.Error())
		mErr.Disable()
	} else {
		for _, p := range profiles {
			profile := p
			item := systray.AddMenuItem("", fmt.Sprintf("Switch to %s", profile.Name))
			a.profItems = append(a.profItems, profileItem{item: item, profile: profile})

			go func() {
				for {
					<-item.ClickedCh
					a.switchProfile(profile)
					// Trigger immediate refresh after switch
					a.refreshMenu()
				}
			}()
		}
	}

	systray.AddSeparator()

	// --- Namespaces ---
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

	// Set initial labels
	a.refreshLabels(active)
}

// refreshMenu updates all dynamic labels. Safe to call from any goroutine.
func (a *app) refreshMenu() {
	a.mu.Lock()
	defer a.mu.Unlock()

	active := a.cm.GetActiveProfile()
	a.refreshLabels(active)
}

// refreshLabels updates the tray title and all menu item labels.
// Must be called with a.mu held.
func (a *app) refreshLabels(active string) {
	region := a.ps.GetDefaultRegion()

	// Tray title
	title := fmt.Sprintf("☁ %s", active)
	if region != "" {
		title += fmt.Sprintf(" (%s)", region)
	}
	systray.SetTitle(title)

	// Status header
	a.mStatus.SetTitle(fmt.Sprintf("Active: %s", active))

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

	// Profile items
	for i := range a.profItems {
		pi := &a.profItems[i]
		pi.profile.IsActive = (pi.profile.Name == active)
		pi.item.SetTitle(a.formatProfileLabel(pi.profile))
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

// formatProfileLabel builds the display label for a profile menu item.
func (a *app) formatProfileLabel(profile aws.Profile) string {
	label := profile.Name
	if profile.IsActive {
		label = "✓ " + label
	} else {
		label = "  " + label
	}

	if profile.IsSSO && a.sm != nil {
		if a.sm.IsLoggedIn(profile.Name) {
			remaining := a.getSessionTimeLeft(profile.Name)
			if remaining != "" {
				label += fmt.Sprintf("  (SSO ✓ %s)", remaining)
			} else {
				label += "  (SSO ✓)"
			}
		} else {
			label += "  (SSO ✗ expired)"
		}
	}

	if profile.Region != "" {
		label += "  " + profile.Region
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

// switchProfile handles switching to a profile from the tray.
// Only triggers SSO login if the session is actually expired.
func (a *app) switchProfile(profile aws.Profile) {
	needsLogin := profile.IsSSO && a.sm != nil && !a.sm.IsLoggedIn(profile.Name)

	if needsLogin {
		fmt.Fprintf(os.Stderr, "SSO session expired for %s, logging in...\n", profile.Name)
		if err := a.sm.Login(profile.Name); err != nil {
			fmt.Fprintf(os.Stderr, "SSO login failed for %s: %v\n", profile.Name, err)
			return
		}
		fmt.Fprintf(os.Stderr, "SSO login successful for %s\n", profile.Name)
	}

	if err := a.ps.SwitchProfile(profile.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to switch to %s: %v\n", profile.Name, err)
		return
	}

	// Switch kube context
	if err := a.km.SwitchContextForEnv(profile.Name); err != nil {
		fmt.Fprintf(os.Stderr, "Kube context switch failed: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "Switched to: %s\n", profile.Name)
}
