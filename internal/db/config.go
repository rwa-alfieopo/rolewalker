package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	env := &Environment{}
	err := r.db.QueryRowContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	svc := &Service{}
	err := r.db.QueryRowContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pm := &PortMapping{}
	err := r.db.QueryRowContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	preset := &ScalingPreset{}
	err := r.db.QueryRowContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	endpoint := &APIEndpoint{}
	err := r.db.QueryRowContext(ctx, `
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
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

// AWSAccount represents an AWS account
type AWSAccount struct {
	ID           int
	AccountID    string
	AccountName  string
	SSOStartURL  sql.NullString
	SSORegion    sql.NullString
	Description  sql.NullString
	Active       bool
}

// AWSRole represents an AWS role within an account
type AWSRole struct {
	ID          int
	AccountID   int
	RoleName    string
	RoleARN     sql.NullString
	ProfileName string
	Region      string
	Description sql.NullString
	Active      bool
}

// UserSession represents an active user session
type UserSession struct {
	ID           int
	RoleID       int
	SessionStart string
	SessionEnd   sql.NullString
	IsActive     bool
}

// GetAWSAccount retrieves an AWS account by account ID
func (r *ConfigRepository) GetAWSAccount(accountID string) (*AWSAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	acc := &AWSAccount{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, account_name, sso_start_url, sso_region, description, active
		FROM aws_accounts
		WHERE account_id = ? AND active = 1
	`, accountID).Scan(&acc.ID, &acc.AccountID, &acc.AccountName, &acc.SSOStartURL, &acc.SSORegion, &acc.Description, &acc.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("AWS account not found: %s", accountID)
	}
	if err != nil {
		return nil, err
	}

	return acc, nil
}

// GetAllAWSAccounts retrieves all active AWS accounts
func (r *ConfigRepository) GetAllAWSAccounts() ([]AWSAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, account_id, account_name, sso_start_url, sso_region, description, active
		FROM aws_accounts
		WHERE active = 1
		ORDER BY account_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []AWSAccount
	for rows.Next() {
		var acc AWSAccount
		if err := rows.Scan(&acc.ID, &acc.AccountID, &acc.AccountName, &acc.SSOStartURL, &acc.SSORegion, &acc.Description, &acc.Active); err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}

// GetRolesByAccount retrieves all roles for an AWS account
func (r *ConfigRepository) GetRolesByAccount(accountID string) ([]AWSRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.account_id, r.role_name, r.role_arn, r.profile_name, r.region, r.description, r.active
		FROM aws_roles r
		JOIN aws_accounts a ON r.account_id = a.id
		WHERE a.account_id = ? AND r.active = 1
		ORDER BY r.role_name
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []AWSRole
	for rows.Next() {
		var role AWSRole
		if err := rows.Scan(&role.ID, &role.AccountID, &role.RoleName, &role.RoleARN, &role.ProfileName, &role.Region, &role.Description, &role.Active); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

// GetRoleByProfileName retrieves a role by its profile name
func (r *ConfigRepository) GetRoleByProfileName(profileName string) (*AWSRole, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := &AWSRole{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, account_id, role_name, role_arn, profile_name, region, description, active
		FROM aws_roles
		WHERE profile_name = ? AND active = 1
	`, profileName).Scan(&role.ID, &role.AccountID, &role.RoleName, &role.RoleARN, &role.ProfileName, &role.Region, &role.Description, &role.Active)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role not found: %s", profileName)
	}
	if err != nil {
		return nil, err
	}

	return role, nil
}

// CreateUserSession creates a new user session and deactivates previous ones
func (r *ConfigRepository) CreateUserSession(roleID int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Deactivate all previous sessions
	_, err = tx.Exec(`
		UPDATE user_sessions 
		SET is_active = 0, session_end = CURRENT_TIMESTAMP
		WHERE is_active = 1
	`)
	if err != nil {
		return err
	}

	// Create new session
	_, err = tx.Exec(`
		INSERT INTO user_sessions (role_id, is_active)
		VALUES (?, 1)
	`, roleID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetActiveSession retrieves the currently active session
func (r *ConfigRepository) GetActiveSession() (*UserSession, *AWSRole, *AWSAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := &UserSession{}
	role := &AWSRole{}
	account := &AWSAccount{}

	err := r.db.QueryRowContext(ctx, `
		SELECT 
			s.id, s.role_id, s.session_start, s.session_end, s.is_active,
			r.id, r.account_id, r.role_name, r.role_arn, r.profile_name, r.region, r.description, r.active,
			a.id, a.account_id, a.account_name, a.sso_start_url, a.sso_region, a.description, a.active
		FROM user_sessions s
		JOIN aws_roles r ON s.role_id = r.id
		JOIN aws_accounts a ON r.account_id = a.id
		WHERE s.is_active = 1
		ORDER BY s.session_start DESC
		LIMIT 1
	`).Scan(
		&session.ID, &session.RoleID, &session.SessionStart, &session.SessionEnd, &session.IsActive,
		&role.ID, &role.AccountID, &role.RoleName, &role.RoleARN, &role.ProfileName, &role.Region, &role.Description, &role.Active,
		&account.ID, &account.AccountID, &account.AccountName, &account.SSOStartURL, &account.SSORegion, &account.Description, &account.Active,
	)

	if err == sql.ErrNoRows {
		return nil, nil, nil, fmt.Errorf("no active session found")
	}
	if err != nil {
		return nil, nil, nil, err
	}

	return session, role, account, nil
}

// AddAWSAccount adds a new AWS account
func (r *ConfigRepository) AddAWSAccount(accountID, accountName, ssoStartURL, ssoRegion, description string) error {
	_, err := r.db.Exec(`
		INSERT INTO aws_accounts (account_id, account_name, sso_start_url, sso_region, description)
		VALUES (?, ?, ?, ?, ?)
	`, accountID, accountName, 
		sql.NullString{String: ssoStartURL, Valid: ssoStartURL != ""},
		sql.NullString{String: ssoRegion, Valid: ssoRegion != ""},
		sql.NullString{String: description, Valid: description != ""})
	return err
}

// AddAWSRole adds a new AWS role
func (r *ConfigRepository) AddAWSRole(accountID int, roleName, roleARN, profileName, region, description string) error {
	_, err := r.db.Exec(`
		INSERT INTO aws_roles (account_id, role_name, role_arn, profile_name, region, description)
		VALUES (?, ?, ?, ?, ?, ?)
	`, accountID, roleName,
		sql.NullString{String: roleARN, Valid: roleARN != ""},
		profileName, region,
		sql.NullString{String: description, Valid: description != ""})
	return err
}
