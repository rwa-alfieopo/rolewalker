package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"rolewalkers/aws"
	appconfig "rolewalkers/internal/config"
)

func (c *CLI) setup(args []string) error {
	cfg := appconfig.Get()
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("rolewalkers setup")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()
	fmt.Println("This will automatically discover your AWS accounts, roles,")
	fmt.Println("and EKS clusters, then configure everything for you.")
	fmt.Println()

	// Get SSO start URL
	startURL := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "https://") {
			startURL = arg
		}
	}

	if startURL == "" {
		fmt.Print("Enter your AWS SSO start URL\n")
		fmt.Print("  (e.g. https://d-9c67711d98.awsapps.com/start/#)\n")
		fmt.Print("  URL: ")
		input, _ := reader.ReadString('\n')
		startURL = strings.TrimSpace(input)
	}

	if startURL == "" {
		return fmt.Errorf("SSO start URL is required")
	}

	// Get SSO region
	ssoRegion := cfg.Region
	fs := ParseFlags(args)
	if r := fs.String("region", ""); r != "" {
		ssoRegion = r
	} else {
		fmt.Printf("  SSO Region [%s]: ", ssoRegion)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			ssoRegion = input
		}
	}

	fmt.Println()

	// Run the setup
	setupMgr := aws.NewSetupManager(c.dbRepo)
	result, err := setupMgr.LoginAndDiscover(startURL, ssoRegion)
	if err != nil {
		return err
	}

	// Write default config file if it doesn't exist
	appconfig.WriteDefault()

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println("Setup complete!")
	fmt.Printf("  Accounts:  %d\n", result.Accounts)
	fmt.Printf("  Roles:     %d\n", result.Roles)
	fmt.Printf("  Profiles:  %d\n", result.Profiles)
	fmt.Printf("  Clusters:  %d\n", result.Clusters)

	if len(result.Errors) > 0 {
		fmt.Println()
		fmt.Println("  Warnings:")
		for _, e := range result.Errors {
			fmt.Printf("    ⚠ %s\n", e)
		}
	}

	fmt.Println()
	fmt.Println("You can now use:")
	fmt.Println("  rw list          # See all profiles")
	fmt.Println("  rw switch dev    # Switch to an environment")
	fmt.Println("  rw tray start    # Start the system tray app")

	return nil
}
