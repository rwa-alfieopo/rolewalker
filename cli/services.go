package cli

import (
	"fmt"
)

func (c *CLI) grpc(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw grpc <service> <env>\n       rw grpc list\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	if args[0] == "list" || args[0] == "ls" {
		fmt.Print(c.grpcManager.ListServices())
		return nil
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: rw grpc <service> <env>\n\nServices: %s\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage",
			c.grpcManager.GetServices())
	}

	return c.grpcManager.Forward(args[0], args[1])
}

func (c *CLI) redis(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw redis connect <env>\n\nSubcommands:\n  connect <env>  Connect to Redis cluster via interactive redis-cli\n\nExamples:\n  rw redis connect dev   # Connect to dev Redis cluster\n  rw redis connect prod  # Connect to prod Redis cluster")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "connect":
		if len(subArgs) < 1 {
			return fmt.Errorf("usage: rw redis connect <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
		}
		return c.redisManager.Connect(subArgs[0])
	default:
		return fmt.Errorf("unknown redis subcommand: %s\nUse: connect", subCmd)
	}
}

func (c *CLI) msk(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw msk <ui|stop> <env>\n\nSubcommands:\n  ui <env>    Start Kafka UI for MSK cluster\n  stop <env>  Stop the Kafka UI pod\n\nExamples:\n  rw msk ui dev              # Start Kafka UI on localhost:8080\n  rw msk ui prod --port 9090 # Start on custom port\n  rw msk stop dev            # Stop the Kafka UI pod")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "ui":
		return c.mskUI(subArgs)
	case "stop":
		if len(subArgs) < 1 {
			return fmt.Errorf("usage: rw msk stop <env>\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
		}
		return c.mskManager.StopUI(subArgs[0])
	default:
		return fmt.Errorf("unknown msk subcommand: %s\nUse: ui, stop", subCmd)
	}
}

func (c *CLI) mskUI(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw msk ui <env> [--port <port>]\n\nEnvironments: snd, dev, sit, preprod, trg, prod, qa, stage")
	}

	fs := ParseFlags(args)
	env := fs.Arg(0)

	if env == "" {
		return fmt.Errorf("environment is required\n\nUsage: rw msk ui <env> [--port <port>]")
	}

	port, err := fs.Int("port", 8080)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", fs.String("port", ""))
	}

	return c.mskManager.StartUI(env, port)
}
