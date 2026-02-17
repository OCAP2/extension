package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithValidConfigFile(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfg := `{
		"logLevel": "debug",
		"defaultTag": "PvP",
		"db": { "host": "10.0.0.1", "port": "5433" }
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(cfg), 0644))

	err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "debug", viper.GetString("logLevel"))
	assert.Equal(t, "PvP", viper.GetString("defaultTag"))
	assert.Equal(t, "10.0.0.1", viper.GetString("db.host"))
	assert.Equal(t, "5433", viper.GetString("db.port"))
}

func TestLoad_DefaultValues(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(`{}`), 0644))

	err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "info", viper.GetString("logLevel"))
	assert.Equal(t, "Op", viper.GetString("defaultTag"))
	assert.Equal(t, "./ocaplogs", viper.GetString("logsDir"))
	assert.Equal(t, "http://localhost:5000/api", viper.GetString("api.serverUrl"))
	assert.Equal(t, "", viper.GetString("api.apiKey"))
	assert.Equal(t, "localhost", viper.GetString("db.host"))
	assert.Equal(t, "5432", viper.GetString("db.port"))
	assert.Equal(t, "postgres", viper.GetString("db.username"))
	assert.Equal(t, "postgres", viper.GetString("db.password"))
	assert.Equal(t, "ocap", viper.GetString("db.database"))
	assert.Equal(t, true, viper.GetBool("graylog.enabled"))
	assert.Equal(t, "localhost:12201", viper.GetString("graylog.address"))
	assert.Equal(t, true, viper.GetBool("logio.enabled"))
	assert.Equal(t, "localhost", viper.GetString("logio.host"))
	assert.Equal(t, "28777", viper.GetString("logio.port"))
	assert.Equal(t, "memory", viper.GetString("storage.type"))
	assert.Equal(t, "./recordings", viper.GetString("storage.memory.outputDir"))
	assert.Equal(t, true, viper.GetBool("storage.memory.compressOutput"))
	assert.Equal(t, "3m", viper.GetString("storage.sqlite.dumpInterval"))
	assert.Equal(t, false, viper.GetBool("otel.enabled"))
	assert.Equal(t, "ocap-recorder", viper.GetString("otel.serviceName"))
	assert.Equal(t, "5s", viper.GetString("otel.batchTimeout"))
	assert.Equal(t, "", viper.GetString("otel.endpoint"))
	assert.Equal(t, true, viper.GetBool("otel.insecure"))
}

func TestLoad_MissingFile(t *testing.T) {
	t.Cleanup(viper.Reset)

	err := Load("/nonexistent/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading config file")
}

func TestGetString(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("testKey", "testValue")
	assert.Equal(t, "testValue", GetString("testKey"))
}

func TestGetInt(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("testInt", 42)
	assert.Equal(t, 42, GetInt("testInt"))
}

func TestGetBool(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("testBool", true)
	assert.Equal(t, true, GetBool("testBool"))
}

func TestGetStorageConfig_Defaults(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(`{}`), 0644))
	require.NoError(t, Load(dir))

	cfg := GetStorageConfig()
	assert.Equal(t, "memory", cfg.Type)
	assert.Equal(t, "./recordings", cfg.Memory.OutputDir)
	assert.Equal(t, true, cfg.Memory.CompressOutput)
	assert.Equal(t, 3*time.Minute, cfg.SQLite.DumpInterval)
}

func TestGetStorageConfig_Override(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfg := `{
		"storage": {
			"type": "sqlite",
			"memory": { "outputDir": "/tmp/out", "compressOutput": false },
			"sqlite": { "dumpInterval": "10m" }
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(cfg), 0644))
	require.NoError(t, Load(dir))

	sc := GetStorageConfig()
	assert.Equal(t, "sqlite", sc.Type)
	assert.Equal(t, "/tmp/out", sc.Memory.OutputDir)
	assert.Equal(t, false, sc.Memory.CompressOutput)
	assert.Equal(t, 10*time.Minute, sc.SQLite.DumpInterval)
}

func TestGetOTelConfig_Defaults(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(`{}`), 0644))
	require.NoError(t, Load(dir))

	cfg := GetOTelConfig()
	assert.Equal(t, false, cfg.Enabled)
	assert.Equal(t, "ocap-recorder", cfg.ServiceName)
	assert.Equal(t, 5*time.Second, cfg.BatchTimeout)
	assert.Equal(t, "", cfg.Endpoint)
	assert.Equal(t, true, cfg.Insecure)
}

func TestGetOTelConfig_Override(t *testing.T) {
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	cfg := `{
		"otel": {
			"enabled": true,
			"serviceName": "my-service",
			"batchTimeout": "30s",
			"endpoint": "localhost:4317",
			"insecure": false
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ocap_recorder.cfg.json"), []byte(cfg), 0644))
	require.NoError(t, Load(dir))

	oc := GetOTelConfig()
	assert.Equal(t, true, oc.Enabled)
	assert.Equal(t, "my-service", oc.ServiceName)
	assert.Equal(t, 30*time.Second, oc.BatchTimeout)
	assert.Equal(t, "localhost:4317", oc.Endpoint)
	assert.Equal(t, false, oc.Insecure)
}
