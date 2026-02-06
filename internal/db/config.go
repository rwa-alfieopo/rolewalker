package db

import (
	"database/sql"
	"fmt"
)

// Environment represents an environment configuration
type Environment struct {
	ID          int
	Name        string
	DisplayName string
	Region      string
	AWSProfile  string
	ClusterName string
	Namespace   string
	Active      bool
}

// Service represents a service configuration
type Service struct {
	ID                int
	Name              string
	DisplayName       string
	ServiceType       string
	DefaultRemotePort int
	Description       sql.NullString
	Active            bool
}

// PortMapping represents a port mapping configuration
type PortMapping struct {
	ID            int
	ServiceID     int
	EnvironmentID int
	LocalPort     int
	RemotePort    int
	Description   sql.NullString
	Active        bool
}

// ScalingPreset represents a scaling preset configuration
type ScalingPreset struct {
	ID          int
	Name        string
	DisplayName string
	MinReplicas int
	MaxReplicas int
	Description sql.NullString
	Active      bool
}

// APIEndpoint represents an API endpoint configuration
type APIEndpoint struct {
	ID          int
	Name        string
	BaseURL     string
	Description sql.NullString
	Active      bool
}

// ConfigRepository provides methods to access configuration data
type ConfigRepository struct {
	db *DB
}

// NewConfigRepository creates a new config repository
func NewConfigRepository(db *DB) *ConfigRepository {
	return &ConfigRepository{db: db}
}

// GetEnvironment retrieves an environment by name
func (r *ConfigRepository) GetEnvironment(name string) (*Environment, error) {
	env := &Environment{}
	err := r.db.QueryRow(`
		SELECT id, name, display_name, region, aws_profile, cluster_name, namespace, active
		FROM environments
		WHERE name = ? AND active = 1
	`, name).Scan(&env.ID, &env.Name, &env.DisplayName, &env.Region, &env.AWSProfile, &env.ClusterName, &env.Namespace, &env.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("environment not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return env, nil
}

// GetAllEnvironments retrieves all active environments
func (r *ConfigRepository) GetAllEnvironments() ([]Environment, error) {
	rows, err := r.db.Query(`
		SELECT id, name, display_name, region, aws_profile, cluster_name, namespace, active
		FROM environments
		WHERE active = 1
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []Environment
	for rows.Next() {
		var env Environment
		if err := rows.Scan(&env.ID, &env.Name, &env.DisplayName, &env.Region, &env.AWSProfile, &env.ClusterName, &env.Namespace, &env.Active); err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}

	return envs, rows.Err()
}

// GetService retrieves a service by name
func (r *ConfigRepository) GetService(name string) (*Service, error) {
	svc := &Service{}
	err := r.db.QueryRow(`
		SELECT id, name, display_name, service_type, default_remote_port, description, active
		FROM services
		WHERE name = ? AND active = 1
	`, name).Scan(&svc.ID, &svc.Name, &svc.DisplayName, &svc.ServiceType, &svc.DefaultRemotePort, &svc.Description, &svc.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("service not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// GetAllServices retrieves all active services
func (r *ConfigRepository) GetAllServices() ([]Service, error) {
	rows, err := r.db.Query(`
		SELECT id, name, display_name, service_type, default_remote_port, description, active
		FROM services
		WHERE active = 1
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var svc Service
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.DisplayName, &svc.ServiceType, &svc.DefaultRemotePort, &svc.Description, &svc.Active); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}

	return services, rows.Err()
}

// GetPortMapping retrieves a port mapping for a service and environment
func (r *ConfigRepository) GetPortMapping(serviceName, envName string) (*PortMapping, error) {
	pm := &PortMapping{}
	err := r.db.QueryRow(`
		SELECT pm.id, pm.service_id, pm.environment_id, pm.local_port, pm.remote_port, pm.description, pm.active
		FROM port_mappings pm
		JOIN services s ON pm.service_id = s.id
		JOIN environments e ON pm.environment_id = e.id
		WHERE s.name = ? AND e.name = ? AND pm.active = 1
	`, serviceName, envName).Scan(&pm.ID, &pm.ServiceID, &pm.EnvironmentID, &pm.LocalPort, &pm.RemotePort, &pm.Description, &pm.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("port mapping not found for service %s in environment %s", serviceName, envName)
	}
	if err != nil {
		return nil, err
	}

	return pm, nil
}

// GetPortMappingsByService retrieves all port mappings for a service
func (r *ConfigRepository) GetPortMappingsByService(serviceName string) ([]PortMapping, error) {
	rows, err := r.db.Query(`
		SELECT pm.id, pm.service_id, pm.environment_id, pm.local_port, pm.remote_port, pm.description, pm.active
		FROM port_mappings pm
		JOIN services s ON pm.service_id = s.id
		WHERE s.name = ? AND pm.active = 1
		ORDER BY pm.local_port
	`, serviceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []PortMapping
	for rows.Next() {
		var pm PortMapping
		if err := rows.Scan(&pm.ID, &pm.ServiceID, &pm.EnvironmentID, &pm.LocalPort, &pm.RemotePort, &pm.Description, &pm.Active); err != nil {
			return nil, err
		}
		mappings = append(mappings, pm)
	}

	return mappings, rows.Err()
}

// GetScalingPreset retrieves a scaling preset by name
func (r *ConfigRepository) GetScalingPreset(name string) (*ScalingPreset, error) {
	preset := &ScalingPreset{}
	err := r.db.QueryRow(`
		SELECT id, name, display_name, min_replicas, max_replicas, description, active
		FROM scaling_presets
		WHERE name = ? AND active = 1
	`, name).Scan(&preset.ID, &preset.Name, &preset.DisplayName, &preset.MinReplicas, &preset.MaxReplicas, &preset.Description, &preset.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scaling preset not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return preset, nil
}

// GetAllScalingPresets retrieves all active scaling presets
func (r *ConfigRepository) GetAllScalingPresets() ([]ScalingPreset, error) {
	rows, err := r.db.Query(`
		SELECT id, name, display_name, min_replicas, max_replicas, description, active
		FROM scaling_presets
		WHERE active = 1
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var presets []ScalingPreset
	for rows.Next() {
		var preset ScalingPreset
		if err := rows.Scan(&preset.ID, &preset.Name, &preset.DisplayName, &preset.MinReplicas, &preset.MaxReplicas, &preset.Description, &preset.Active); err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}

	return presets, rows.Err()
}

// GetAPIEndpoint retrieves an API endpoint by name
func (r *ConfigRepository) GetAPIEndpoint(name string) (*APIEndpoint, error) {
	endpoint := &APIEndpoint{}
	err := r.db.QueryRow(`
		SELECT id, name, base_url, description, active
		FROM api_endpoints
		WHERE name = ? AND active = 1
	`, name).Scan(&endpoint.ID, &endpoint.Name, &endpoint.BaseURL, &endpoint.Description, &endpoint.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("API endpoint not found: %s", name)
	}
	if err != nil {
		return nil, err
	}

	return endpoint, nil
}

// GetGRPCMicroservices retrieves all gRPC microservices
func (r *ConfigRepository) GetGRPCMicroservices() (map[string]int, error) {
	rows, err := r.db.Query(`
		SELECT name, default_remote_port
		FROM services
		WHERE service_type = 'grpc-microservice' AND active = 1
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	microservices := make(map[string]int)
	for rows.Next() {
		var name string
		var port int
		if err := rows.Scan(&name, &port); err != nil {
			return nil, err
		}
		// Remove "grpc-" prefix from name
		if len(name) > 5 && name[:5] == "grpc-" {
			name = name[5:]
		}
		microservices[name] = port
	}

	return microservices, rows.Err()
}
