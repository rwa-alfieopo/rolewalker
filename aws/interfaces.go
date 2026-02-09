package aws

import (
	"rolewalkers/internal/db"
	"time"
)

// --- Core abstractions (DIP) ---

// ProfileProvider reads AWS profile configurations.
type ProfileProvider interface {
	GetProfiles() ([]Profile, error)
	GetActiveProfile() string
}

// ProfileSwitcherI switches the active AWS profile.
type ProfileSwitcherI interface {
	SwitchProfile(profileName string) error
	GetDefaultRegion() string
	ExportEnvironment(profileName string) (map[string]string, error)
	GenerateShellExport(profileName string, shell string) (string, error)
	ClearEnvironment(shell string) string
}

// SSOManagerI handles SSO login/logout and status.
type SSOManagerI interface {
	Login(profileName string) error
	Logout(profileName string) error
	IsLoggedIn(profileName string) bool
	GetSSOProfiles() ([]Profile, error)
	GetCredentialExpiry(profileName string) (*time.Time, error)
	ValidateProfile(profileName string) error
}

// KubeManagerI handles Kubernetes context operations.
type KubeManagerI interface {
	GetContexts() ([]KubeContext, error)
	GetCurrentContext() (string, error)
	GetCurrentNamespace() string
	SetNamespace(namespace string) error
	ListNamespaces() ([]string, error)
	SwitchContext(contextName string) error
	SwitchContextForEnv(env string) error
	SwitchContextForEnvWithProfile(env string, profileSwitcher *ProfileSwitcher) error
	GetProfileNameForEnv(env string) string
	ListContextsFormatted() (string, error)
}

// EndpointResolver retrieves service endpoints from SSM.
type EndpointResolver interface {
	GetParameter(name string) (string, error)
	GetEndpoint(env, service string) (string, error)
	GetDatabaseEndpoint(env, nodeType, dbType string) (string, error)
	ListParameters(prefix string) ([]string, error)
}

// TunnelManagerI manages tunnel lifecycle.
type TunnelManagerI interface {
	Start(config TunnelConfig) error
	Stop(service, env string) error
	StopAll() error
	List() string
	CleanupStale() error
	GetSupportedServices() string
}

// DatabaseManagerI handles database connection operations.
type DatabaseManagerI interface {
	Connect(config DatabaseConfig) error
	Backup(config BackupConfig) error
	Restore(config RestoreConfig) error
}

// GRPCManagerI handles gRPC port-forwarding.
type GRPCManagerI interface {
	Forward(service, env string) error
	GetServices() string
	ListServices() string
}

// RedisManagerI handles Redis connections.
type RedisManagerI interface {
	Connect(env string) error
}

// MSKManagerI handles MSK Kafka UI operations.
type MSKManagerI interface {
	StartUI(env string, localPort int) error
	StopUI(env string) error
}

// MaintenanceManagerI handles Fastly maintenance mode.
type MaintenanceManagerI interface {
	Toggle(env, serviceType string, enable bool) error
	Status(env string) ([]MaintenanceStatus, error)
}

// ScalingManagerI handles HPA scaling operations.
type ScalingManagerI interface {
	Scale(env, presetName string) error
	ScaleService(env, service string, min, max int) error
	ListHPAs(env string) (string, error)
}

// ReplicationManagerI handles Blue-Green deployment operations.
type ReplicationManagerI interface {
	Status(env string) (string, error)
	Switch(env, deploymentID string) error
	Create(env, name, source string) error
	Delete(deploymentID string, deleteTarget bool) error
}

// ConfigSyncI handles config file ↔ database synchronization.
type ConfigSyncI interface {
	ConfigFileExists() bool
	HasExistingData() bool
	SyncConfigToDB() (*SyncResult, error)
	AnalyzeSync() (*SyncResult, error)
	WriteAWSConfig() error
	BackupConfigFile() (string, error)
	DeleteConfigFile() error
	GetConfigPath() string
}

// --- Consumer-scoped interfaces (ISP) ---

// ScalingConfigProvider is the narrow interface ScalingManager needs.
type ScalingConfigProvider interface {
	GetScalingPreset(name string) (*db.ScalingPreset, error)
	GetAllScalingPresets() ([]db.ScalingPreset, error)
	GetAllEnvironments() ([]db.Environment, error)
}

// EnvironmentProvider is the narrow interface for environment lookups.
type EnvironmentProvider interface {
	GetEnvironment(name string) (*db.Environment, error)
	GetAllEnvironments() ([]db.Environment, error)
}

// ServiceProvider is the narrow interface for service lookups.
type ServiceProvider interface {
	GetService(name string) (*db.Service, error)
	GetAllServices() ([]db.Service, error)
	GetGRPCMicroservices() (map[string]int, error)
}

// PortMappingProvider is the narrow interface for port mapping lookups.
type PortMappingProvider interface {
	GetPortMapping(serviceName, envName string) (*db.PortMapping, error)
}

// AccountRoleProvider is the narrow interface for account/role operations.
type AccountRoleProvider interface {
	GetAllAWSAccounts() ([]db.AWSAccount, error)
	GetAWSAccount(accountID string) (*db.AWSAccount, error)
	GetRolesByAccount(accountID string) ([]db.AWSRole, error)
	GetRoleByProfileName(profileName string) (*db.AWSRole, error)
	GetAllAWSRoles() ([]db.AWSRole, error)
	AddAWSAccount(accountID, accountName, ssoStartURL, ssoRegion, description string) error
	AddAWSRole(accountID int, roleName, roleARN, profileName, region, description string) error
	CreateUserSession(roleID int) error
	GetActiveSession() (*db.UserSession, *db.AWSRole, *db.AWSAccount, error)
}
