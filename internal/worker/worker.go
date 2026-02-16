package worker

import (
	"fmt"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/parser"
	"github.com/OCAP2/extension/v5/internal/storage"
)

// ErrTooEarlyForStateAssociation is returned when state data arrives before entity is registered
var ErrTooEarlyForStateAssociation = fmt.Errorf("too early for state association")

// Dependencies holds all dependencies for the worker manager
type Dependencies struct {
	EntityCache   *cache.EntityCache
	MarkerCache   *cache.MarkerCache
	LogManager    *logging.SlogManager
	ParserService parser.Service
}

// Manager manages worker goroutines
type Manager struct {
	deps    Dependencies
	backend storage.Backend
}

// NewManager creates a new worker manager
func NewManager(deps Dependencies, backend storage.Backend) *Manager {
	return &Manager{
		deps:    deps,
		backend: backend,
	}
}

