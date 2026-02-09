package cli

import "fmt"

func (c *CLI) ssm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw ssm <get|list> <path>\n\nSubcommands:\n  get <path>     Get parameter value\n  list <prefix>  List parameters under prefix\n\nExamples:\n  rw ssm get /dev/zenith/database/query/db-write-endpoint\n  rw ssm get /prod/zenith/redis/cluster-endpoint --decrypt\n  rw ssm list /dev/zenith/")
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "get":
		return c.ssmGet(subArgs)
	case "list", "ls":
		return c.ssmList(subArgs)
	default:
		return fmt.Errorf("unknown ssm subcommand: %s\nUse: get, list", subCmd)
	}
}

func (c *CLI) ssmGet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw ssm get <path> [--decrypt]\n\nExamples:\n  rw ssm get /dev/zenith/database/query/db-write-endpoint\n  rw ssm get /prod/zenith/redis/cluster-endpoint")
	}

	value, err := c.ssmManager.GetParameter(args[0])
	if err != nil {
		return err
	}

	fmt.Println(value)
	return nil
}

func (c *CLI) ssmList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: rw ssm list <prefix>\n\nExamples:\n  rw ssm list /dev/zenith/\n  rw ssm list /prod/zenith/database/")
	}

	prefix := args[0]
	params, err := c.ssmManager.ListParameters(prefix)
	if err != nil {
		return err
	}

	if len(params) == 0 {
		fmt.Printf("No parameters found under: %s\n", prefix)
		return nil
	}

	fmt.Printf("Parameters under %s:\n", prefix)
	for _, p := range params {
		fmt.Printf("  %s\n", p)
	}

	return nil
}
