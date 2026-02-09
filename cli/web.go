package cli

import (
	"fmt"
	"rolewalkers/aws"
	"rolewalkers/internal/web"
	"strconv"
)

func (c *CLI) web(args []string) error {
	port := 8080

	for i, arg := range args {
		if arg == "--port" && i+1 < len(args) {
			p, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i+1])
			}
			port = p
		}
	}

	cm, err := aws.NewConfigManager()
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}

	roleSwitcher := aws.NewRoleSwitcher(cm, c.dbRepo)

	server := web.NewServer(port, c.dbRepo, roleSwitcher)
	return server.Start()
}
