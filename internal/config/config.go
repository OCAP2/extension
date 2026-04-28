package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// MemoryConfig holds in-memory/JSON storage backend settings
type MemoryConfig struct {
	OutputDir      string `json:"outputDir" mapstructure:"outputDir"`
	CompressOutput bool   `json:"compressOutput" mapstructure:"compressOutput"`
}

// SQLiteConfig holds SQLite storage backend settings
type SQLiteConfig struct {
	DumpInterval time.Duration `json:"dumpInterval" mapstructure:"dumpInterval"`
}

// PostgresConfig holds PostgreSQL storage backend connection settings.
type PostgresConfig struct {
	Host     string `json:"host" mapstructure:"host"`
	Port     string `json:"port" mapstructure:"port"`
	Username string `json:"username" mapstructure:"username"`
	Password string `json:"password" mapstructure:"password"`
	Database string `json:"database" mapstructure:"database"`
}

// Load reads configuration from JSON file and sets default values.
// configDir is the directory containing the config file.
func Load(configDir string) error {
	// Set default values
	viper.SetDefault("logLevel", "info")
	viper.SetDefault("defaultTag", "Op")
	viper.SetDefault("logsDir", "./ocaplogs")

	viper.SetDefault("api.serverUrl", "http://localhost:5000")
	viper.SetDefault("api.apiKey", "")
	// 10 minute default — generous enough for multi-hundred-MB uploads across
	// a reverse proxy without being so long that a dead backend hangs the save
	// worker forever.
	viper.SetDefault("api.uploadTimeout", "10m")

	viper.SetDefault("graylog.enabled", true)
	viper.SetDefault("graylog.address", "localhost:12201")

	viper.SetDefault("logio.enabled", true)
	viper.SetDefault("logio.host", "localhost")
	viper.SetDefault("logio.port", "28777")

	// Storage backend defaults
	viper.SetDefault("storage.type", "memory")
	viper.SetDefault("storage.memory.outputDir", "./recordings")
	viper.SetDefault("storage.memory.compressOutput", true)
	viper.SetDefault("storage.sqlite.dumpInterval", "3m")
	viper.SetDefault("storage.postgres.host", "localhost")
	viper.SetDefault("storage.postgres.port", "5432")
	viper.SetDefault("storage.postgres.username", "postgres")
	viper.SetDefault("storage.postgres.password", "postgres")
	viper.SetDefault("storage.postgres.database", "ocap")

	// OpenTelemetry defaults
	viper.SetDefault("otel.enabled", false)
	viper.SetDefault("otel.serviceName", "ocap-recorder")
	viper.SetDefault("otel.batchTimeout", "5s")
	viper.SetDefault("otel.endpoint", "")    // OTLP endpoint (optional)
	viper.SetDefault("otel.insecure", true)  // Use insecure for OTLP

	viper.SetConfigName("ocap_recorder.cfg.json")
	viper.AddConfigPath(configDir)
	viper.SetConfigType("json")

	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	return nil
}

// GetString returns a string config value.
func GetString(key string) string {
	return viper.GetString(key)
}

// GetInt returns an int config value.
func GetInt(key string) int {
	return viper.GetInt(key)
}

// GetBool returns a bool config value.
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	Type     string         `json:"type" mapstructure:"type"`
	Memory   MemoryConfig   `json:"memory" mapstructure:"memory"`
	SQLite   SQLiteConfig   `json:"sqlite" mapstructure:"sqlite"`
	Postgres PostgresConfig `json:"postgres" mapstructure:"postgres"`
}

// GetStorageConfig returns the storage backend configuration
func GetStorageConfig() StorageConfig {
	var cfg StorageConfig
	if err := viper.UnmarshalKey("storage", &cfg); err != nil {
		cfg.Type = "memory"
	}
	return cfg
}

// OTelConfig holds OpenTelemetry configuration
type OTelConfig struct {
	Enabled      bool          `json:"enabled" mapstructure:"enabled"`
	ServiceName  string        `json:"serviceName" mapstructure:"serviceName"`
	BatchTimeout time.Duration `json:"batchTimeout" mapstructure:"batchTimeout"`
	Endpoint     string        `json:"endpoint" mapstructure:"endpoint"`   // OTLP endpoint (optional)
	Insecure     bool          `json:"insecure" mapstructure:"insecure"`   // Use insecure for OTLP
}

// GetOTelConfig returns the OpenTelemetry configuration
func GetOTelConfig() OTelConfig {
	var cfg OTelConfig
	_ = viper.UnmarshalKey("otel", &cfg)
	return cfg
}

// APIConfig holds HTTP client configuration for the OCAP web API.
type APIConfig struct {
	ServerURL     string        `json:"serverUrl" mapstructure:"serverUrl"`
	APIKey        string        `json:"apiKey" mapstructure:"apiKey"`
	UploadTimeout time.Duration `json:"uploadTimeout" mapstructure:"uploadTimeout"`
}

// GetAPIConfig returns the HTTP API client configuration.
func GetAPIConfig() APIConfig {
	var cfg APIConfig
	_ = viper.UnmarshalKey("api", &cfg)
	return cfg
}
