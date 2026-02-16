// Package sqlitestorage implements the storage.Backend interface using an in-memory
// SQLite database with periodic disk dumps via VACUUM INTO.
// It wraps the GORM backend via composition — the only SQLite-specific concerns are:
// (a) creating the in-memory DB, (b) skipping ProjectileEvent (no PostGIS),
// (c) periodic disk dump, and (d) schema migration without PostGIS.
package sqlitestorage

import (
	"fmt"
	"time"

	"github.com/OCAP2/extension/v5/internal/cache"
	"github.com/OCAP2/extension/v5/internal/database"
	"github.com/OCAP2/extension/v5/internal/logging"
	"github.com/OCAP2/extension/v5/internal/mission"
	"github.com/OCAP2/extension/v5/pkg/core"
	gormstorage "github.com/OCAP2/extension/v5/internal/storage/gorm"

	"gorm.io/gorm"
)

// Config holds configuration for the SQLite storage backend.
type Config struct {
	DumpInterval time.Duration
	DumpPath     string // Path for periodic VACUUM INTO dumps
}

// Backend wraps the GORM backend for SQLite-specific behavior.
type Backend struct {
	*gormstorage.Backend
	db       *gorm.DB
	cfg      Config
	log      *logging.SlogManager
	stopChan chan struct{}
}

// New creates a new SQLite storage backend.
func New(cfg Config, entityCache *cache.EntityCache, markerCache *cache.MarkerCache, logManager *logging.SlogManager, missionCtx *mission.Context) (*Backend, error) {
	db, err := database.GetSqliteDBStandalone("")
	if err != nil {
		return nil, fmt.Errorf("failed to create in-memory SQLite DB: %w", err)
	}

	gormBackend := gormstorage.New(gormstorage.Dependencies{
		DB:             db,
		EntityCache:    entityCache,
		MarkerCache:    markerCache,
		LogManager:     logManager,
		MissionContext: missionCtx,
	})

	return &Backend{
		Backend:  gormBackend,
		db:       db,
		cfg:      cfg,
		log:      logManager,
		stopChan: make(chan struct{}),
	}, nil
}

// Init initializes the embedded GORM backend and starts the dump goroutine.
func (b *Backend) Init() error {
	if err := b.Backend.Init(); err != nil {
		return err
	}

	if b.cfg.DumpPath != "" && b.cfg.DumpInterval > 0 {
		go b.dumpLoop()
	}

	return nil
}

// Close stops the dump goroutine and closes the embedded GORM backend.
func (b *Backend) Close() error {
	close(b.stopChan)
	return b.Backend.Close()
}

// RecordProjectileEvent is a no-op — SQLite doesn't support LineStringZM (PostGIS geometry).
func (b *Backend) RecordProjectileEvent(e *core.ProjectileEvent) error {
	return nil
}

// dumpLoop periodically dumps the in-memory SQLite database to disk via VACUUM INTO.
// VACUUM INTO creates a point-in-time snapshot, so no pause mechanism is needed.
func (b *Backend) dumpLoop() {
	ticker := time.NewTicker(b.cfg.DumpInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopChan:
			return
		case <-ticker.C:
			start := time.Now()
			if err := database.DumpMemoryDBToDisk(b.db, b.cfg.DumpPath); err != nil {
				b.log.WriteLog("sqlite:dumpLoop", fmt.Sprintf("Error dumping to disk: %v", err), "ERROR")
			} else {
				b.log.WriteLog("sqlite:dumpLoop", fmt.Sprintf("Dumped to disk in %s", time.Since(start)), "DEBUG")
			}
		}
	}
}
