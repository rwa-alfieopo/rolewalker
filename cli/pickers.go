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
	tunnels := c.tunnelManager.ListTunnels()
	if len(tunnels) == 0 {
		return nil, fmt.Errorf("no active tunnels to stop\nUse 'rw tunnel start <service> <env>' to create one")
	}

	// Build labels for the picker
	labels := make([]string, len(tunnels))
	for i, t := range tunnels {
		labels[i] = fmt.Sprintf("%s-%s  (localhost:%d → %s)", t.Service, t.Environment, t.LocalPort, t.RemoteHost)
	}

	selected, ok := utils.SelectFromList("Select tunnel to stop:", labels)
	if !ok {
		return nil, fmt.Errorf("selection cancelled")
	}

	// Find the matching tunnel
	for i, label := range labels {
		if label == selected {
			return &tunnelChoice{Service: tunnels[i].Service, Environment: tunnels[i].Environment}, nil
		}
	}

	return nil, fmt.Errorf("unexpected selection error")
}
