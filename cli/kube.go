package cli

import (
	"fmt"
	"rolewalkers/internal/utils"
	"strings"
)

func (c *CLI) kube(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw kube <env>\n       rw kube list\n       rw kube set namespace\n\nExamples:\n  rw kube dev              # Switch to dev EKS cluster context\n  rw kube prod             # Switch to prod EKS cluster context\n  rw kube list             # List all available contexts\n  rw kube set namespace    # Interactively set default namespace")
	}

	subCmd := args[0]

	if subCmd == "list" || subCmd == "ls" {
		output, err := c.kubeManager.ListContextsFormatted()
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	}

	if subCmd == "current" {
		ctx, err := c.kubeManager.GetCurrentContext()
		if err != nil {
			return err
		}
		fmt.Printf("Current kubectl context: %s\n", ctx)
		return nil
	}

	if subCmd == "set" {
		if len(args) < 2 {
			return fmt.Errorf("usage: rw kube set namespace")
		}
		if args[1] == "namespace" || args[1] == "ns" {
			return c.kubeSetNamespace()
		}
		return fmt.Errorf("unknown set option: %s\nUse: namespace", args[1])
	}

	// Otherwise treat as environment name
	env := subCmd
	profileName := c.kubeManager.GetProfileNameForEnv(env)

	if err := c.profileSwitcher.SwitchProfile(profileName); err != nil {
		return fmt.Errorf("failed to switch AWS profile: %w", err)
	}

	if err := c.kubeManager.SwitchContextForEnvWithProfile(env, c.profileSwitcher); err != nil {
		return err
	}

	namespace := c.kubeManager.GetCurrentNamespace()
	if namespace == "" {
		namespace = "default"
	}

	fmt.Println()
	return c.showKubeContext(namespace)
}

func (c *CLI) kubeSetNamespace() error {
	namespaces, err := c.kubeManager.ListNamespaces()
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		return fmt.Errorf("no namespaces found in current cluster")
	}

	selectedNS, ok := utils.SelectFromList("Available namespaces:", namespaces)
	if !ok {
		fmt.Println("Namespace selection cancelled.")
		return nil
	}

	if err := c.kubeManager.SetNamespace(selectedNS); err != nil {
		return fmt.Errorf("failed to set namespace: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Namespace set successfully!")
	fmt.Println()

	return c.showKubeContext(selectedNS)
}

func (c *CLI) showKubeContext(namespace string) error {
	activeProfile := c.configManager.GetActiveProfile()
	region := c.profileSwitcher.GetDefaultRegion()

	fmt.Println("Current Context:")
	fmt.Println(strings.Repeat("-", 60))

	fmt.Printf("AWS Profile:     %s\n", activeProfile)
	if region != "" {
		fmt.Printf("AWS Region:      %s\n", region)
	}

	profiles, err := c.configManager.GetProfiles()
	if err == nil {
		for _, p := range profiles {
			if p.Name == activeProfile && p.IsSSO {
				fmt.Printf("Account ID:      %s\n", p.SSOAccountID)
				accountName := c.extractAccountName(p.Name)
				if accountName != "" {
					fmt.Printf("Account Name:    %s\n", accountName)
				}
				break
			}
		}
	}

	kubeContext, err := c.kubeManager.GetCurrentContext()
	if err == nil && kubeContext != "" {
		fmt.Printf("Kube Cluster:    %s\n", kubeContext)
		fmt.Printf("Kube Namespace:  %s\n", namespace)
	} else {
		fmt.Printf("Kube Cluster:    (not configured)\n")
		fmt.Printf("Kube Namespace:  (not configured)\n")
	}

	return nil
}
