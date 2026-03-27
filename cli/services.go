package cli

import (
	"fmt"
)

func (c *CLI) grpc(args []string) error {
	if len(args) >= 1 && (args[0] == "list" || args[0] == "ls") {
		fmt.Print(c.grpcManager.ListServices())
		return nil
	}

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
			picked, err := c.pickService(true)
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

	return c.grpcManager.Forward(service, env)
}

func (c *CLI) redis(args []string) error {
	if len(args) >= 1 && args[0] == "connect" {
		if len(args) >= 2 {
			return c.redisManager.Connect(args[1])
		}
		// Interactive environment picker
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		return c.redisManager.Connect(picked)
	}

	if len(args) < 1 {
		// No args at all — default to connect with picker
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		return c.redisManager.Connect(picked)
	}

	return fmt.Errorf("unknown redis subcommand: %s\nUse: connect", args[0])
}

func (c *CLI) msk(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw msk <ui|connect|stop> <env>\n\nSubcommands:\n  ui <env>      Start Kafka UI for MSK cluster\n  connect <env> Start interactive Kafka CLI session (IAM auth)\n  stop <env>    Stop the Kafka UI pod\n\nExamples:\n  rw msk ui dev              # Start Kafka UI on localhost:8080\n  rw msk ui prod --port 9090 # Start on custom port\n  rw msk connect dev         # Interactive Kafka CLI\n  rw msk stop dev            # Stop the Kafka UI pod")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "ui":
		return c.mskUI(subArgs)
	case "connect", "cli":
		return c.mskConnect(subArgs)
	case "stop":
		env := ""
		if len(subArgs) >= 1 {
			env = subArgs[0]
		} else {
			picked, err := c.pickEnvironment()
			if err != nil {
				return err
			}
			env = picked
		}
		return c.mskManager.StopUI(env)
	default:
		return fmt.Errorf("unknown msk subcommand: %s\nUse: ui, connect, stop", subCmd)
	}
}

func (c *CLI) mskUI(args []string) error {
	fs := ParseFlags(args)
	env := fs.Arg(0)

	if env == "" {
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		env = picked
	}

	port, err := fs.Int("port", 8080)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", fs.String("port", ""))
	}

	return c.mskManager.StartUI(env, port)
}

func (c *CLI) mskConnect(args []string) error {
	var env string
	if len(args) >= 1 {
		env = args[0]
	} else {
		picked, err := c.pickEnvironment()
		if err != nil {
			return err
		}
		env = picked
	}

	return c.mskManager.ConnectCLI(env)
}
