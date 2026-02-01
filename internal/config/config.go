package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	Type     string         `json:"type" mapstructure:"type"`
	Postgres PostgresConfig `json:"postgres" mapstructure:"postgres"`
	SQLite   SQLiteConfig   `json:"sqlite" mapstructure:"sqlite"`
	Memory   MemoryConfig   `json:"memory" mapstructure:"memory"`
}

// PostgresConfig holds PostgreSQL connection settings
type PostgresConfig struct {
	Host     string `json:"host" mapstructure:"host"`
	Port     string `json:"port" mapstructure:"port"`
	Username string `json:"username" mapstructure:"username"`
	Password string `json:"password" mapstructure:"password"`
	Database string `json:"database" mapstructure:"database"`
}

// SQLiteConfig holds SQLite settings
type SQLiteConfig struct {
	Path         string `json:"path" mapstructure:"path"`
	InMemory     bool   `json:"inMemory" mapstructure:"inMemory"`
	DumpInterval string `json:"dumpInterval" mapstructure:"dumpInterval"`
}

// MemoryConfig holds in-memory/JSON backend settings
type MemoryConfig struct {
	OutputDir      string `json:"outputDir" mapstructure:"outputDir"`
	CompressOutput bool   `json:"compressOutput" mapstructure:"compressOutput"`
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

	viper.SetDefault("db.host", "localhost")
	viper.SetDefault("db.port", "5432")
	viper.SetDefault("db.username", "postgres")
	viper.SetDefault("db.password", "postgres")
	viper.SetDefault("db.database", "ocap")

	viper.SetDefault("influx.enabled", true)
	viper.SetDefault("influx.host", "localhost")
	viper.SetDefault("influx.port", "8086")
	viper.SetDefault("influx.protocol", "http")
	viper.SetDefault("influx.token", "supersecrettoken")
	viper.SetDefault("influx.org", "ocap-metrics")

	viper.SetDefault("graylog.enabled", true)
	viper.SetDefault("graylog.address", "localhost:12201")

	viper.SetDefault("logio.enabled", true)
	viper.SetDefault("logio.host", "localhost")
	viper.SetDefault("logio.port", "28777")

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
