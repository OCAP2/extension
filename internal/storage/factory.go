// internal/storage/factory.go
package storage

import (
	"fmt"

	"github.com/OCAP2/extension/internal/config"
)

// NewBackend creates a storage backend based on configuration
func NewBackend(cfg config.StorageConfig) (Backend, error) {
	switch cfg.Type {
	case "postgres":
		return nil, fmt.Errorf("postgres backend not yet implemented")
	case "sqlite":
		return nil, fmt.Errorf("sqlite backend not yet implemented")
	case "memory":
		return nil, fmt.Errorf("memory backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}
