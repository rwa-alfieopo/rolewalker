package aws

import (
	"fmt"
	"rolewalkers/internal/db"
	"slices"
	"strings"
)

// ServicePortMapping defines local ports for services per environment
type ServicePortMapping struct {
	Service     string
	Environment string
	Port        int
	Description string
}

// PortConfig holds all port mappings
type PortConfig struct {
	configRepo *db.ConfigRepository
}

// NewPortConfig creates a new port configuration
func NewPortConfig() *PortConfig {
	return &PortConfig{configRepo: nil}
}

// NewPortConfigWithRepo creates a new port configuration with a shared config repository
func NewPortConfigWithRepo(repo *db.ConfigRepository) *PortConfig {
	return &PortConfig{configRepo: repo}
}

// GetPort returns the port(s) for a service and environment
func (pc *PortConfig) GetPort(service, env string) ([]int, error) {
	service = strings.ToLower(service)
	env = strings.ToLower(env)

	if pc.configRepo != nil {
		pm, err := pc.configRepo.GetPortMapping(service, env)
		if err == nil {
			return []int{pm.LocalPort}, nil
		}
	}

	return nil, fmt.Errorf("port mapping not found for service: %s in environment: %s", service, env)
}

// GetServices returns all available services
func (pc *PortConfig) GetServices() string {
	if pc.configRepo != nil {
		services, err := pc.configRepo.GetAllServices()
		if err == nil {
			names := make([]string, len(services))
			for i, s := range services {
				names[i] = s.Name
			}
			slices.Sort(names)
			return strings.Join(names, ", ")
		}
	}
	return DefaultServices
}

// GetEnvironments returns all available environments
func (pc *PortConfig) GetEnvironments() string {
	if pc.configRepo != nil {
		envs, err := pc.configRepo.GetAllEnvironments()
		if err == nil {
			names := make([]string, len(envs))
			for i, e := range envs {
				names[i] = e.Name
			}
			slices.Sort(names)
			return strings.Join(names, ", ")
		}
	}
	return strings.Join(DefaultEnvironments, ", ")
}

// ListAll returns a formatted string of all port mappings
func (pc *PortConfig) ListAll() string {
	var sb strings.Builder

	sb.WriteString("Port Mappings:\n")
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	if pc.configRepo != nil {
		services, err := pc.configRepo.GetAllServices()
		if err == nil {
			envs, err := pc.configRepo.GetAllEnvironments()
			if err == nil {
				for _, service := range services {
					if service.ServiceType == "grpc-microservice" {
						continue // Skip microservices in main listing
					}
					fmt.Fprintf(&sb, "\n%s:\n", strings.ToUpper(service.Name))
					for _, env := range envs {
						pm, err := pc.configRepo.GetPortMapping(service.Name, env.Name)
						if err == nil {
							fmt.Fprintf(&sb, "  %-8s %d\n", env.Name+":", pm.LocalPort)
						}
					}
				}
				return sb.String()
			}
		}
	}

	// Fallback to legacy format
	sb.WriteString("Database not available. Please initialize the database.\n")
	return sb.String()
}
