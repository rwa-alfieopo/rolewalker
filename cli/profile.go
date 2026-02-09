package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func (c *CLI) listProfiles() error {
	profiles, err := c.configManager.GetProfiles()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Println("No AWS profiles found.")
		return nil
	}

	fmt.Println("AWS Profiles:")
	fmt.Println(strings.Repeat("-", 80))

	for _, p := range profiles {
		status := ""
		if p.IsActive {
			status = " [ACTIVE]"
		}

		ssoStatus := ""
		if p.IsSSO {
			if c.ssoManager.IsLoggedIn(p.Name) {
				ssoStatus = " (SSO: logged in)"
			} else {
				ssoStatus = " (SSO: not logged in)"
			}
		}

		fmt.Printf("  %s%s%s\n", p.Name, status, ssoStatus)

		if p.Region != "" {
			fmt.Printf("    Region: %s\n", p.Region)
		}
		if p.IsSSO {
			fmt.Printf("    Account: %s | Role: %s\n", p.SSOAccountID, p.SSORoleName)
		}
	}

	return nil
}

func (c *CLI) switchProfile(profileName string, skipKube bool) error {
	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		return err
	}

	if !skipKube {
		if err := c.kubeManager.SwitchContextForEnv(profileName); err != nil {
			fmt.Printf("⚠ Failed to switch kubectl context: %v\n", err)
		}
	}

	namespace := c.kubeManager.GetCurrentNamespace()
	if namespace == "" {
		namespace = "default"
	}

	fmt.Println()
	c.showKubeContext(namespace)

	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" && envProfile != profileName {
		fmt.Println("\n⚠ AWS_PROFILE environment variable is set and overrides the config.")
		fmt.Println("  Clear it for this terminal:")
		if runtime.GOOS == "windows" {
			fmt.Println("    Remove-Item Env:AWS_PROFILE")
		} else {
			fmt.Println("    unset AWS_PROFILE")
		}
		fmt.Println("  (New terminals will work automatically)")
	}

	return nil
}

func (c *CLI) login(profileName string) error {
	fmt.Printf("Initiating SSO login for profile: %s\n", profileName)
	fmt.Println("A browser window will open for authentication...")

	if err := c.ssoManager.Login(profileName); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	fmt.Printf("✓ Successfully logged in to: %s\n", profileName)

	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		fmt.Printf("⚠ Logged in but could not set default profile: %v\n", err)
		fmt.Printf("  Run 'rw switch %s' manually, or use --profile %s\n", profileName, profileName)
	}

	return nil
}

func (c *CLI) logout(profileName string) error {
	if err := c.ssoManager.Logout(profileName); err != nil {
		return fmt.Errorf("logout failed: %w", err)
	}

	fmt.Printf("✓ Logged out from: %s\n", profileName)
	return nil
}

func (c *CLI) status() error {
	profiles, err := c.ssoManager.GetSSOProfiles()
	if err != nil {
		return err
	}

	if len(profiles) == 0 {
		fmt.Println("No SSO profiles configured.")
		return nil
	}

	fmt.Println("SSO Profile Status:")
	fmt.Println(strings.Repeat("-", 60))

	for _, p := range profiles {
		status := "✗ Not logged in"
		if c.ssoManager.IsLoggedIn(p.Name) {
			status = "✓ Logged in"
			if expiry, err := c.ssoManager.GetCredentialExpiry(p.Name); err == nil {
				status += fmt.Sprintf(" (expires: %s)", expiry.Format("15:04:05"))
			}
		}

		active := ""
		if p.IsActive {
			active = " [ACTIVE]"
		}

		fmt.Printf("  %s%s: %s\n", p.Name, active, status)
	}

	return nil
}

func (c *CLI) current() error {
	namespace := c.kubeManager.GetCurrentNamespace()
	if namespace == "" {
		namespace = "default"
	}

	if err := c.showKubeContext(namespace); err != nil {
		return err
	}

	active := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()

	if envProfile := os.Getenv("AWS_PROFILE"); envProfile != "" && envProfile != active {
		fmt.Printf("\n⚠ AWS_PROFILE env override: %s\n", envProfile)
	}
	if envRegion := os.Getenv("AWS_DEFAULT_REGION"); envRegion != "" && envRegion != region {
		fmt.Printf("⚠ AWS_DEFAULT_REGION env override: %s\n", envRegion)
	}

	return nil
}

func (c *CLI) context(args []string) error {
	fs := ParseFlags(args)
	format := fs.String("format", "default")

	activeProfile := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()

	accountID := ""
	accountName := ""
	profiles, err := c.configManager.GetProfiles()
	if err == nil {
		for _, p := range profiles {
			if p.Name == activeProfile && p.IsSSO {
				accountID = p.SSOAccountID
				accountName = c.extractAccountName(p.Name)
				break
			}
		}
	}

	kubeContext := ""
	namespace := ""
	if ctx, err := c.kubeManager.GetCurrentContext(); err == nil {
		kubeContext = ctx
		if strings.Contains(kubeContext, "/") {
			parts := strings.Split(kubeContext, "/")
			kubeContext = parts[len(parts)-1]
		}
	}

	ns := c.kubeManager.GetCurrentNamespace()
	if ns != "" {
		namespace = ns
	} else {
		namespace = "default"
	}

	switch format {
	case "short":
		fmt.Printf("%s|%s|%s|%s\n", activeProfile, accountName, kubeContext, namespace)

	case "json":
		jsonOutput := map[string]string{
			"profile":      activeProfile,
			"account_name": accountName,
			"account_id":   accountID,
			"region":       region,
			"eks_cluster":  kubeContext,
			"namespace":    namespace,
		}
		if err := json.NewEncoder(os.Stdout).Encode(jsonOutput); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}

	default:
		fmt.Printf("Profile:   %s\n", activeProfile)
		if accountName != "" {
			fmt.Printf("Account:   %s", accountName)
			if accountID != "" {
				fmt.Printf(" (%s)", accountID)
			}
			fmt.Println()
		}
		if region != "" {
			fmt.Printf("Region:    %s\n", region)
		}
		if kubeContext != "" {
			fmt.Printf("EKS:       %s\n", kubeContext)
			fmt.Printf("Namespace: %s\n", namespace)
		}
	}

	return nil
}
