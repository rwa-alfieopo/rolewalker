package aws

import (
	"fmt"
	"sort"
	"strings"
)

// PortMapping defines local ports for services per environment
type PortMapping struct {
	Service     string
	Environment string
	Port        int
	Description string
}

// PortConfig holds all port mappings
type PortConfig struct {
	mappings map[string]map[string][]int // service -> env -> ports
}

// NewPortConfig creates a new port configuration with predefined mappings
func NewPortConfig() *PortConfig {
	pc := &PortConfig{
		mappings: make(map[string]map[string][]int),
	}
	pc.initMappings()
	return pc
}

func (pc *PortConfig) initMappings() {
	// Database ports
	pc.mappings["db"] = map[string][]int{
		"snd":     {5432},
		"dev":     {5433},
		"sit":     {5434},
		"preprod": {5435, 5436},
		"trg":     {5437},
		"prod":    {5438, 5439},
		"qa":      {5440, 5441},
		"stage":   {5442, 5443},
	}

	// Redis ports
	pc.mappings["redis"] = map[string][]int{
		"snd":     {6379},
		"dev":     {6380},
		"sit":     {6381},
		"preprod": {6382},
		"trg":     {6383},
		"prod":    {6384},
		"qa":      {6385},
		"stage":   {6386},
	}

	// Elasticsearch ports (default for all envs)
	pc.mappings["elasticsearch"] = map[string][]int{
		"snd":     {9200},
		"dev":     {9200},
		"sit":     {9200},
		"preprod": {9200},
		"trg":     {9200},
		"prod":    {9200},
		"qa":      {9200},
		"stage":   {9200},
	}

	// Kafka ports
	pc.mappings["kafka"] = map[string][]int{
		"snd":     {9092},
		"dev":     {9093},
		"sit":     {9094},
		"preprod": {9095},
		"trg":     {9096},
		"prod":    {9097},
		"qa":      {9098},
		"stage":   {9099},
	}

	// RabbitMQ ports
	pc.mappings["rabbitmq"] = map[string][]int{
		"snd":     {5672},
		"dev":     {5673},
		"sit":     {5674},
		"preprod": {5675},
		"trg":     {5676},
		"prod":    {5677},
		"qa":      {5678},
		"stage":   {5679},
	}

	// gRPC ports
	pc.mappings["grpc"] = map[string][]int{
		"snd":     {50051},
		"dev":     {50052},
		"sit":     {50053},
		"preprod": {50054},
		"trg":     {50055},
		"prod":    {50056},
		"qa":      {50057},
		"stage":   {50058},
	}
}

// GetPort returns the port(s) for a service and environment
func (pc *PortConfig) GetPort(service, env string) ([]int, error) {
	service = strings.ToLower(service)
	env = strings.ToLower(env)

	serviceMap, ok := pc.mappings[service]
	if !ok {
		return nil, fmt.Errorf("unknown service: %s\nAvailable: %s", service, pc.GetServices())
	}

	ports, ok := serviceMap[env]
	if !ok {
		return nil, fmt.Errorf("unknown environment: %s\nAvailable: %s", env, pc.GetEnvironments())
	}

	return ports, nil
}

// GetServices returns all available services
func (pc *PortConfig) GetServices() string {
	services := make([]string, 0, len(pc.mappings))
	for s := range pc.mappings {
		services = append(services, s)
	}
	sort.Strings(services)
	return strings.Join(services, ", ")
}

// GetEnvironments returns all available environments
func (pc *PortConfig) GetEnvironments() string {
	return "snd, dev, sit, preprod, trg, prod, qa, stage"
}

// ListAll returns a formatted string of all port mappings
func (pc *PortConfig) ListAll() string {
	var sb strings.Builder

	services := []string{"db", "redis", "elasticsearch", "kafka", "rabbitmq", "grpc"}
	envs := []string{"snd", "dev", "sit", "preprod", "trg", "prod", "qa", "stage"}

	sb.WriteString("Port Mappings:\n")
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	for _, service := range services {
		sb.WriteString(fmt.Sprintf("\n%s:\n", strings.ToUpper(service)))
		for _, env := range envs {
			if ports, err := pc.GetPort(service, env); err == nil {
				portStrs := make([]string, len(ports))
				for i, p := range ports {
					portStrs[i] = fmt.Sprintf("%d", p)
				}
				sb.WriteString(fmt.Sprintf("  %-8s %s\n", env+":", strings.Join(portStrs, ", ")))
			}
		}
	}

	return sb.String()
}
