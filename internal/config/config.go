// Package config provides application-wide settings loaded from
// ~/.rolewalkers/config.yaml. Every value has a sensible default so the
// tool works out of the box without a config file.
package config

import (
	"rolewalkers/internal/utils"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all configurable settings for the application.
type Config struct {
	// Project is the project name used in SSM paths, kube namespaces, etc.
	Project string `yaml:"project"`

	// Region is the default AWS region.
	Region string `yaml:"region"`

	// SSMPathPrefix is the template for SSM parameter paths.
	// Use {env} and {project} as placeholders.
	// e.g. "/{env}/{project}" → "/dev/zenith"
	SSMPathPrefix string `yaml:"ssm_path_prefix"`

	// Namespaces configures Kubernetes namespace settings.
	Namespaces NamespaceConfig `yaml:"namespaces"`

	// Database configures database connection settings.
	Database DatabaseConfig `yaml:"database"`

	// Images configures container images used for pods.
	Images ImageConfig `yaml:"images"`

	// ProfilePrefix is the prefix for AWS profile names (e.g. "zenith-").
	ProfilePrefix string `yaml:"profile_prefix"`

	// ProductionEnvs lists environment names that require confirmation prompts.
	ProductionEnvs []string `yaml:"production_envs"`

	// ProdLikeEnvs lists environments that have separate query/command DB clusters.
	ProdLikeEnvs []string `yaml:"prod_like_envs"`
}

// NamespaceConfig holds Kubernetes namespace settings.
type NamespaceConfig struct {
	// App is the main application namespace (default: "zenith").
	App string `yaml:"app"`

	// Tunnel is the namespace for tunnel/temp pods (default: "tunnel-access").
	Tunnel string `yaml:"tunnel"`

	// QuickSwitch lists namespaces shown in the tray and pickers.
	QuickSwitch []string `yaml:"quick_switch"`
}

// DatabaseConfig holds database-related settings.
type DatabaseConfig struct {
	// MasterUser is the admin DB username (default: "zenithmaster").
	MasterUser string `yaml:"master_user"`

	// ReadOnlyUser is the read-only IAM DB username (default: "zenith-ro").
	ReadOnlyUser string `yaml:"readonly_user"`

	// AdminUser is the admin IAM DB username (default: "zenith-admin").
	AdminUser string `yaml:"admin_user"`

	// Port is the default PostgreSQL port (default: 5432).
	Port int `yaml:"port"`

	// DefaultDB is the default database name to connect to (default: "postgres").
	DefaultDB string `yaml:"default_db"`

	// RedisUser is the Redis auth username (default: "zenithmaster").
	RedisUser string `yaml:"redis_user"`

	// RedisPort is the default Redis port (default: 6379).
	RedisPort int `yaml:"redis_port"`
}

// ImageConfig holds container image references.
type ImageConfig struct {
	Postgres string `yaml:"postgres"`
	Redis    string `yaml:"redis"`
	Socat    string `yaml:"socat"`
	KafkaCLI string `yaml:"kafka_cli"`
	KafkaUI  string `yaml:"kafka_ui"`
}

// Defaults returns a Config with all default values.
func Defaults() *Config {
	return &Config{
		Project:       "zenith",
		Region:        "eu-west-2",
		SSMPathPrefix: "/{env}/{project}",
		ProfilePrefix: "zenith-",
		ProductionEnvs: []string{"prod", "preprod", "trg", "live"},
		ProdLikeEnvs:   []string{"prod", "qa", "stage", "preprod", "trg"},
		Namespaces: NamespaceConfig{
			App:         "zenith",
			Tunnel:      "tunnel-access",
			QuickSwitch: []string{"zenith", "tunnel-access", "default", "kube-system"},
		},
		Database: DatabaseConfig{
			MasterUser:   "zenithmaster",
			ReadOnlyUser: "zenith-ro",
			AdminUser:    "zenith-admin",
			Port:         5432,
			DefaultDB:    "postgres",
			RedisUser:    "zenithmaster",
			RedisPort:    6379,
		},
		Images: ImageConfig{
			Postgres: "postgres:15-alpine",
			Redis:    "redis:7-alpine",
			Socat:    "alpine/socat",
			KafkaCLI: "confluentinc/cp-kafka:7.7.6",
			KafkaUI:  "provectuslabs/kafka-ui:latest",
		},
	}
}

const configFileName = "config.yaml"

var (
	globalCfg  *Config
	globalOnce sync.Once
)

// Load reads the config file from ~/.rolewalkers/config.yaml.
// Missing fields use defaults. If the file doesn't exist, all defaults are used.
func Load() *Config {
	globalOnce.Do(func() {
		globalCfg = Defaults()

		data, err := utils.ReadRoleWalkersFile(configFileName)
		if err != nil {
			return // File doesn't exist — use defaults
		}

		// Unmarshal on top of defaults so missing fields keep their default values
		_ = yaml.Unmarshal(data, globalCfg)
	})
	return globalCfg
}

// Get returns the global config (loads on first call).
func Get() *Config {
	return Load()
}

// SSMPath builds an SSM parameter path using the configured prefix.
// e.g. SSMPath("dev", "database/query/db-read-endpoint")
// → "/dev/zenith/database/query/db-read-endpoint"
func (c *Config) SSMPath(env, suffix string) string {
	prefix := c.SSMPathPrefix
	prefix = replaceAll(prefix, "{env}", env)
	prefix = replaceAll(prefix, "{project}", c.Project)
	return prefix + "/" + suffix
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// IsProductionEnv checks if the given environment is in the production list.
func (c *Config) IsProductionEnv(env string) bool {
	for _, e := range c.ProductionEnvs {
		if e == env {
			return true
		}
	}
	return false
}

// IsProdLikeEnv checks if the environment has separate query/command clusters.
func (c *Config) IsProdLikeEnv(env string) bool {
	for _, e := range c.ProdLikeEnvs {
		if e == env {
			return true
		}
	}
	return false
}

// WriteDefault writes a default config file to ~/.rolewalkers/config.yaml
// if one doesn't already exist.
func WriteDefault() error {
	if _, err := utils.ReadRoleWalkersFile(configFileName); err == nil {
		return nil // Already exists
	}

	data, err := yaml.Marshal(Defaults())
	if err != nil {
		return err
	}

	header := []byte("# rolewalkers configuration\n# Edit this file to customize settings.\n# All values shown are defaults — remove a line to use the default.\n\n")
	return utils.WriteRoleWalkersFile(configFileName, append(header, data...))
}

// Reset resets the global config singleton (for testing).
func Reset() {
	globalOnce = sync.Once{}
	globalCfg = nil
}
