package cli

import (
	"fmt"
	"rolewalkers/aws"
)

func (c *CLI) tunnel(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw tunnel <start|stop|list> [service] [env]\n\nSubcommands:\n  start <service> <env>  Start a tunnel\n  stop <service> <env>   Stop a specific tunnel\n  stop --all             Stop all tunnels\n  list                   List active tunnels\n  cleanup                Remove stale tunnel entries\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage", c.tunnelManager.GetSupportedServices())
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "start":
		return c.tunnelStart(subArgs)
	case "stop":
		return c.tunnelStop(subArgs)
	case "list", "ls":
		fmt.Print(c.tunnelManager.List())
		return nil
	case "cleanup":
		return c.tunnelManager.CleanupStale()
	default:
		return fmt.Errorf("unknown tunnel subcommand: %s\nUse: start, stop, list, cleanup", subCmd)
	}
}

func (c *CLI) tunnelStart(args []string) error {
	service := ""
	env := ""

	if len(args) >= 2 {
		service = args[0]
		env = args[1]
	} else {
		// Interactive picker for missing arguments
		if len(args) >= 1 {
			service = args[0]
		} else {
			picked, err := c.pickService(false)
			if err != nil {
				return err
			}
			service = picked
		}
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		env = picked
	}

	config := aws.TunnelConfig{
		Service:     service,
		Environment: env,
		NodeType:    "read",
		DBType:      "query",
	}

	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--write", "-w":
			config.NodeType = "write"
		case "--command", "-c":
			config.DBType = "command"
		}
	}

	return c.tunnelManager.Start(config)
}

func (c *CLI) tunnelStop(args []string) error {
	if len(args) > 0 && (args[0] == "--all" || args[0] == "-a") {
		return c.tunnelManager.StopAll()
	}

	if len(args) < 2 {
		// Interactive: pick from active tunnels
		picked, err := c.pickActiveTunnel()
		if err != nil {
			return err
		}
		return c.tunnelManager.Stop(picked.Service, picked.Environment)
	}

	service := args[0]
	env := args[1]

	return c.tunnelManager.Stop(service, env)
}

func (c *CLI) port(args []string) error {
	portConfig := aws.NewPortConfigWithRepo(c.dbRepo)

	if len(args) > 0 && (args[0] == "--list" || args[0] == "-l" || args[0] == "list" || args[0] == "ls") {
		fmt.Print(portConfig.ListAll())
		return nil
	}

	service := ""
	env := ""

	if len(args) >= 2 {
		service = args[0]
		env = args[1]
	} else {
		if len(args) >= 1 {
			service = args[0]
		} else {
			picked, err := c.pickService(false)
			if err != nil {
				return err
			}
			service = picked
		}
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		env = picked
	}

	ports, err := portConfig.GetPort(service, env)
	if err != nil {
		return err
	}

	for i, p := range ports {
		if i > 0 {
			fmt.Print("/")
		}
		fmt.Print(p)
	}
	fmt.Println()

	return nil
}
