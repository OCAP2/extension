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

// Load reads configuration from JSON file and sets default values.
// configDir is the directory containing the config file.
func Load(configDir string) error {
	// Set default values
	viper.SetDefault("logLevel", "info")
	viper.SetDefault("defaultTag", "Op")
	viper.SetDefault("logsDir", "./ocaplogs")

	viper.SetDefault("api.serverUrl", "http://localhost:5000/api")
	viper.SetDefault("api.apiKey", "")

	viper.SetDefault("db.host", "localhost")
	viper.SetDefault("db.port", "5432")
	viper.SetDefault("db.username", "postgres")
	viper.SetDefault("db.password", "postgres")
	viper.SetDefault("db.database", "ocap")

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
	Type   string       `json:"type" mapstructure:"type"`
	Memory MemoryConfig `json:"memory" mapstructure:"memory"`
	SQLite SQLiteConfig `json:"sqlite" mapstructure:"sqlite"`
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
