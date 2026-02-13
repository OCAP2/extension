# Go Idiom Refactoring Opportunities

This document tracks opportunities to improve the codebase using idiomatic Go patterns.

## Status

| # | Refactoring | Status | PR |
|---|-------------|--------|-----|
| 1 | Generic Queues | ✅ Done | [#48](https://github.com/OCAP2/extension/pull/48) |
| 2 | DB Writer Helper | ✅ Done | [#49](https://github.com/OCAP2/extension/pull/49) |
| 3 | Global State → DI | Pending | - |
| 4 | Functional Options | Pending | - |
| 5 | Error Wrapping | Pending | - |
| 6 | Interface Segregation | Pending | - |

---

## High Impact / Low Effort

### 1. Generic Queues ✅

**Location:** `internal/queue/queue.go`

**Problem:** 16+ identical queue implementations with copy-pasted code.

**Solution:** Single generic `Queue[T]` using Go 1.18+ generics.

**Result:** 1006 → 114 lines (-892 lines, 89% reduction)

---

### 2. DB Writer Helper ✅

**Location:** `internal/worker/worker.go`

**Problem:** Same transaction pattern repeated 16 times:
```go
if !m.queues.X.Empty() {
    tx := m.deps.DB.Begin()
    toWrite := m.queues.X.GetAndEmpty()
    err := tx.Create(&toWrite).Error
    if err != nil {
        log(...)
        tx.Rollback()
    } else {
        tx.Commit()
        // optional post-write callback
    }
}
```

**Solution:** Generic `writeQueue[T]` helper function.

**Result:** 356 → 186 lines (-170 lines, 48% reduction)

---

## High Impact / Medium Effort

### 3. Global State → Dependency Injection

**Location:** `cmd/ocap_recorder/main.go`

**Problem:** 10+ package-level variables create hidden dependencies:
```go
var (
    DB              *gorm.DB
    ShouldSaveLocal bool = false
    IsDatabaseValid bool = false
    EntityCache     *cache.EntityCache = cache.NewEntityCache()
    MarkerCache     *cache.MarkerCache = cache.NewMarkerCache()
    SlogManager     *logging.SlogManager
    // ... more globals
)
```

**Issues:**
- Hard to test (global side effects)
- Unclear data flow
- No concurrent instance support
- Callbacks set dynamically make dependency graph unclear

**Solution:** Create `AppContext` struct with explicit dependencies:
```go
type AppContext struct {
    db              *gorm.DB
    entityCache     *cache.EntityCache
    markerCache     *cache.MarkerCache
    logger          *slog.Logger
    logManager      *logging.SlogManager
    shouldSaveLocal bool
    isDatabaseValid bool
}

func (app *AppContext) StartGoroutines() error {
    handlerService := handlers.NewService(
        handlers.WithDB(app.db),
        handlers.WithLogger(app.logger),
    )
    // ...
}
```

**Benefits:**
- Explicit dependency injection
- Easier testing (no global state)
- Multiple concurrent instances possible
- Clearer data flow

---

### 4. Functional Options Pattern

**Location:** Multiple services (`worker`, `handlers`, `monitor`)

**Problem:** Large `Dependencies` structs with many fields:
```go
type Dependencies struct {
    DB              *gorm.DB
    EntityCache     *cache.EntityCache
    MarkerCache     *cache.MarkerCache
    LogManager      *logging.SlogManager
    HandlerService  *handlers.Service
    IsDatabaseValid func() bool
    ShouldSaveLocal func() bool
    DBInsertsPaused func() bool
}

func NewManager(deps Dependencies, queues *Queues) *Manager
```

**Solution:** Functional options pattern:
```go
type Option func(*Manager)

func WithDB(db *gorm.DB) Option {
    return func(m *Manager) { m.db = db }
}

func WithEntityCache(cache *cache.EntityCache) Option {
    return func(m *Manager) { m.entityCache = cache }
}

func NewManager(opts ...Option) *Manager {
    m := &Manager{}
    for _, opt := range opts {
        opt(m)
    }
    return m
}

// Usage
mgr := worker.NewManager(
    worker.WithDB(db),
    worker.WithEntityCache(cache),
    worker.WithLogger(logger),
)
```

**Benefits:**
- No intermediate struct types
- Easy to add new options without breaking existing code
- Supports partial initialization for testing
- More discoverable API (each option is a function)
- Common Go pattern (used by `gorm.DB`, `http.Server`, etc.)

---

## Medium Impact / Low Effort

### 5. Error Wrapping Consistency

**Location:** Multiple files

**Problem:** Inconsistent error wrapping styles:
```go
// Using %s (loses error chain)
return nil, fmt.Errorf("failed to get local SQLite DB: %s", err)

// Using %v (loses error chain)
return nil, fmt.Errorf("database connection is nil")

// Correct: using %w (preserves error chain)
return nil, fmt.Errorf("failed to create resource: %w", err)
```

**Solution:** Standardize on `%w` for all error wrapping:
```go
return nil, fmt.Errorf("failed to get local SQLite DB: %w", err)
return nil, fmt.Errorf("failed to connect to database: %w", err)
```

**Benefits:**
- Preserves error chain for `errors.Is()` and `errors.As()`
- Consistent debugging experience
- Recommended in Go 1.13+

**Files to update:**
- `cmd/ocap_recorder/main.go`
- `internal/handlers/handlers.go`
- `internal/database/database.go`

---

### 6. Storage Interface Segregation

**Location:** `internal/storage/storage.go`

**Problem:** Large `Backend` interface with 24+ methods:
```go
type Backend interface {
    Init() error
    Close() error
    StartMission(mission *core.Mission, world *core.World) error
    EndMission() error
    AddSoldier(s *core.Soldier) error
    AddVehicle(v *core.Vehicle) error
    AddMarker(m *core.Marker) error
    RecordSoldierState(s *core.SoldierState) error
    RecordVehicleState(v *core.VehicleState) error
    // ... 15+ more methods
}
```

**Solution:** Split into focused interfaces:
```go
// Lifecycle management
type Lifecycle interface {
    Init() error
    Close() error
}

// Mission management
type MissionManager interface {
    StartMission(mission *core.Mission, world *core.World) error
    EndMission() error
}

// Entity registration
type EntityRegistry interface {
    AddSoldier(s *core.Soldier) error
    AddVehicle(v *core.Vehicle) error
    AddMarker(m *core.Marker) error
}

// State recording
type StateRecorder interface {
    RecordSoldierState(s *core.SoldierState) error
    RecordVehicleState(v *core.VehicleState) error
    RecordMarkerState(s *core.MarkerState) error
}

// Event recording
type EventRecorder interface {
    RecordFiredEvent(e *core.FiredEvent) error
    RecordGeneralEvent(e *core.GeneralEvent) error
    RecordHitEvent(e *core.HitEvent) error
    RecordKillEvent(e *core.KillEvent) error
    // ...
}

// Cache lookups
type EntityLookup interface {
    GetSoldierByOcapID(ocapID uint16) (*core.Soldier, bool)
    GetVehicleByOcapID(ocapID uint16) (*core.Vehicle, bool)
    GetMarkerByName(name string) (*core.Marker, bool)
}

// Full backend composes all interfaces
type Backend interface {
    Lifecycle
    MissionManager
    EntityRegistry
    StateRecorder
    EventRecorder
    EntityLookup
}
```

**Benefits:**
- Handlers can accept only what they need
- Easier to test with mock implementations
- Better API contract clarity
- Follows Interface Segregation Principle

---

## Additional Opportunities (Lower Priority)

### Context Propagation

**Location:** Dispatcher/handler chain

**Problem:** No `context.Context` passing through handlers:
```go
type HandlerFunc func(Event) (any, error)  // No context
```

**Solution:** Add context support:
```go
type HandlerFunc func(context.Context, Event) (any, error)
```

**Benefits:**
- Enables graceful shutdown
- Supports operation timeouts
- Allows passing cancellation signals

---

### Embedded Interface for Logging

**Location:** `internal/logging/slog.go`

**Problem:** Manual callback delegation for state:
```go
type SlogManager struct {
    GetMissionName  func() string
    GetMissionID    func() uint
    IsUsingLocalDB  func() bool
    IsStatusRunning func() bool
}
```

**Solution:** Define and embed a `StateProvider` interface:
```go
type StateProvider interface {
    MissionName() string
    MissionID() uint
    IsUsingLocalDB() bool
    IsStatusRunning() bool
}

type SlogManager struct {
    logger        *slog.Logger
    StateProvider StateProvider
}
```

**Benefits:**
- Separates concerns
- Explicit interface contract
- Easier to test with mocks
