package cli

import (
	"fmt"
	"rolewalkers/aws"
	"rolewalkers/internal/utils"
	"strings"
)

// tunnelChoice holds the result of an active tunnel selection.
type tunnelChoice struct {
	Service     string
	Environment string
}

// pickService shows an interactive service picker.
// If grpcOnly is true, only gRPC microservices are shown.
func (c *CLI) pickService(grpcOnly bool) (string, error) {
	var items []string

	if c.dbRepo != nil {
		if grpcOnly {
			grpcServices, err := c.dbRepo.GetGRPCMicroservices()
			if err == nil && len(grpcServices) > 0 {
				for name := range grpcServices {
					items = append(items, name)
				}
			}
		} else {
			services, err := c.dbRepo.GetAllServices()
			if err == nil {
				for _, s := range services {
					if s.ServiceType != "grpc-microservice" {
						items = append(items, s.Name)
					}
				}
			}
		}
	}

	// Fallback to defaults if DB unavailable or empty
	if len(items) == 0 {
		if grpcOnly {
			items = strings.Split(aws.DefaultGRPCServices, ", ")
		} else {
			items = strings.Split(aws.DefaultServices, ", ")
		}
		for i := range items {
			items[i] = strings.TrimSpace(items[i])
		}
	}

	if len(items) == 0 {
		return "", fmt.Errorf("no services available")
	}

	selected, ok := utils.SelectFromList("Select a service:", items)
	if !ok {
		return "", fmt.Errorf("selection cancelled")
	}
	return selected, nil
}

// pickEnvironment shows an interactive environment picker.
func (c *CLI) pickEnvironment() (string, error) {
	var items []string

	if c.dbRepo != nil {
		envs, err := c.dbRepo.GetAllEnvironments()
		if err == nil && len(envs) > 0 {
			for _, e := range envs {
				items = append(items, e.Name)
			}
		}
	}

	// Fallback to defaults if DB unavailable or empty
	if len(items) == 0 {
		items = aws.DefaultEnvironments
	}

	if len(items) == 0 {
		return "", fmt.Errorf("no environments available")
	}

	selected, ok := utils.SelectFromList("Select an environment:", items)
	if !ok {
		return "", fmt.Errorf("selection cancelled")
	}
	return selected, nil
}

// pickActiveTunnel shows a picker of currently active tunnels for stopping.
func (c *CLI) pickActiveTunnel() (*tunnelChoice, error) {
	listOutput := c.tunnelManager.List()
	if strings.Contains(listOutput, "No active tunnels") || strings.TrimSpace(listOutput) == "" {
		return nil, fmt.Errorf("no active tunnels to stop\nUse 'rw tunnel start <service> <env>' to create one")
	}

	// Show the list and fall back to service+env picker
	fmt.Print(listOutput)
	fmt.Println()

	fmt.Println("Pick the tunnel to stop:")
	service, err := c.pickService(false)
	if err != nil {
		return nil, err
	}
	env, err := c.pickEnvironment()
	if err != nil {
		return nil, err
	}

	return &tunnelChoice{Service: service, Environment: env}, nil
}
