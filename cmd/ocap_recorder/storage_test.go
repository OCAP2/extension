package main

import (
	"log/slog"
	"testing"

	"github.com/OCAP2/extension/v5/internal/config"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/internal/storage/websocket"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCreateWebSocketBackend(t *testing.T) {
	viper.Set("api.serverUrl", "https://example.test/ocap/")
	viper.Set("api.apiKey", "test-key")
	t.Cleanup(viper.Reset)
	b, err := createStorageBackend(config.StorageConfig{Type: "websocket"})
	require.NoError(t, err)
	require.IsType(t, (*websocket.Backend)(nil), b)
	_, uploads := b.(storage.Uploadable)
	require.False(t, uploads, "streaming proxy must not export or upload local recordings")
}

func TestStorageDefaultsRemainCompatible(t *testing.T) {
	previous := Logger
	Logger = slog.Default()
	t.Cleanup(func() { Logger = previous })
	b, err := createStorageBackend(config.StorageConfig{Type: "memory"})
	require.NoError(t, err)
	_, uploads := b.(storage.Uploadable)
	require.True(t, uploads)
	_, err = createStorageBackend(config.StorageConfig{Type: "not-real"})
	require.Error(t, err)
}

func TestWebSocketBackendRequiresSecret(t *testing.T) {
	viper.Set("api.apiKey", "")
	t.Cleanup(viper.Reset)
	_, err := createStorageBackend(config.StorageConfig{Type: "websocket"})
	require.Error(t, err)
}

func TestWebSocketURLPreservesPrefix(t *testing.T) {
	require.Equal(t, "wss://example.test/ocap", httpToWS("https://example.test/ocap/"))
}
