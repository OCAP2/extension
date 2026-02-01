# Tier 2 Extraction Plan: Refactoring main.go

## Overview

Following the successful Tier 1 extraction (config, database, influx packages), this plan outlines the next phase of refactoring to further reduce `cmd/ocap_recorder/main.go` from ~4,844 lines to a maintainable size.

## Current State (Post Tier 1)

```
cmd/ocap_recorder/main.go    4,844 lines, 55+ functions
internal/
  config/config.go              65 lines
  database/database.go         342 lines
  influx/influx.go             256 lines
  cache/cache.go                93 lines (existing)
  geo/geo.go                    XX lines (existing)
  model/model.go               XXX lines (existing)
  queue/queue.go               XXX lines (existing)
```

## Proposed New Packages

### 1. `internal/logging` (~150 lines)

Centralize logging setup and utilities.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `setupLogging` | ~65 | Configure zerolog with multi-writer |
| `writeToLogIo` | ~20 | Send logs to log.io service |
| `removeOldLogs` | ~25 | Clean up old log files |
| `writeLog` | ~40 | Unified logging helper |

**Proposed API:**
```go
package logging

type Manager struct {
    Logger      zerolog.Logger
    JSONLogger  zerolog.Logger
    TraceSample zerolog.Logger
    LogIoConn   net.Conn
    GraylogWriter *gelf.Writer
}

func NewManager() *Manager
func (m *Manager) Setup(file *os.File, logLevel string) error
func (m *Manager) WriteLog(function, data, level string)
func (m *Manager) RemoveOldLogs(path string, days int)
```

---

### 2. `internal/handlers` (~1,500 lines)

Event processing and data transformation from Arma format to database models.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `logNewMission` | ~150 | Process new mission data |
| `logNewSoldier` | ~60 | Process new soldier entity |
| `logSoldierState` | ~120 | Process soldier state update |
| `logNewVehicle` | ~40 | Process new vehicle entity |
| `logVehicleState` | ~115 | Process vehicle state update |
| `logFiredEvent` | ~70 | Process weapon fired event |
| `logProjectileEvent` | ~200 | Process projectile tracking |
| `logGeneralEvent` | ~50 | Process general events |
| `logHitEvent` | ~75 | Process hit events |
| `logKillEvent` | ~80 | Process kill events |
| `logChatEvent` | ~75 | Process chat messages |
| `logRadioEvent` | ~70 | Process radio transmissions |
| `logFpsEvent` | ~40 | Process FPS metrics |
| `logAce3DeathEvent` | ~55 | Process ACE3 death events |
| `logAce3UnconsciousEvent` | ~40 | Process ACE3 unconscious events |
| `logMarkerCreate` | ~95 | Process marker creation |
| `logMarkerMove` | ~70 | Process marker movement |
| `logMarkerDelete` | ~25 | Process marker deletion |

**Proposed API:**
```go
package handlers

type Context struct {
    DB           *gorm.DB
    EntityCache  *cache.EntityCache
    MarkerCache  map[string]uint
    Mission      *model.Mission
    Logger       zerolog.Logger
}

func NewContext(db *gorm.DB, cache *cache.EntityCache, logger zerolog.Logger) *Context

// Entity handlers
func (c *Context) ProcessNewMission(data []string) (*model.Mission, error)
func (c *Context) ProcessNewSoldier(data []string) (*model.Soldier, error)
func (c *Context) ProcessSoldierState(data []string) (*model.SoldierState, error)
func (c *Context) ProcessNewVehicle(data []string) (*model.Vehicle, error)
func (c *Context) ProcessVehicleState(data []string) (*model.VehicleState, error)

// Event handlers
func (c *Context) ProcessFiredEvent(data []string) (*model.FiredEvent, error)
func (c *Context) ProcessProjectileEvent(data []string) (*model.ProjectileEvent, error)
func (c *Context) ProcessGeneralEvent(data []string) (*model.GeneralEvent, error)
func (c *Context) ProcessHitEvent(data []string) (*model.HitEvent, error)
func (c *Context) ProcessKillEvent(data []string) (*model.KillEvent, error)
func (c *Context) ProcessChatEvent(data []string) (*model.ChatEvent, error)
func (c *Context) ProcessRadioEvent(data []string) (*model.RadioEvent, error)
func (c *Context) ProcessFpsEvent(data []string) (*model.ServerFpsEvent, error)

// ACE3 handlers
func (c *Context) ProcessAce3DeathEvent(data []string) (*model.Ace3DeathEvent, error)
func (c *Context) ProcessAce3UnconsciousEvent(data []string) (*model.Ace3UnconsciousEvent, error)

// Marker handlers
func (c *Context) ProcessMarkerCreate(data []string) (*model.Marker, error)
func (c *Context) ProcessMarkerMove(data []string) (*model.MarkerState, error)
func (c *Context) ProcessMarkerDelete(data []string) (string, uint, error)
```

---

### 3. `internal/worker` (~500 lines)

Background goroutines for async processing and database writes.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `startGoroutines` | ~60 | Initialize all background workers |
| `startAsyncProcessors` | ~370 | Channel processors for incoming data |
| `startDBWriters` | ~260 | Batch database writers |

**Proposed API:**
```go
package worker

type Pool struct {
    Handlers     *handlers.Context
    InfluxMgr    *influx.Manager
    Queues       *QueueSet
    Channels     map[string]chan []string
    Logger       zerolog.Logger
    StopChan     chan struct{}
}

type QueueSet struct {
    Soldiers              *queue.SoldiersQueue
    SoldierStates         *queue.SoldierStatesQueue
    Vehicles              *queue.VehiclesQueue
    VehicleStates         *queue.VehicleStatesQueue
    // ... other queues
}

func NewPool(ctx *handlers.Context, influx *influx.Manager, logger zerolog.Logger) *Pool
func (p *Pool) Start() error
func (p *Pool) Stop()
func (p *Pool) StartAsyncProcessors()
func (p *Pool) StartDBWriters()
```

---

### 4. `internal/monitor` (~200 lines)

Status monitoring and performance tracking.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `startStatusMonitor` | ~135 | Periodic status reporting |
| `getProgramStatus` | ~80 | Collect current program state |
| `validateHypertables` | ~75 | TimescaleDB hypertable setup |

**Proposed API:**
```go
package monitor

type StatusMonitor struct {
    DB            *gorm.DB
    InfluxMgr     *influx.Manager
    Queues        *worker.QueueSet
    Channels      map[string]chan []string
    Logger        zerolog.Logger
    Interval      time.Duration
    StopChan      chan struct{}
}

func NewStatusMonitor(db *gorm.DB, influx *influx.Manager, logger zerolog.Logger) *StatusMonitor
func (m *StatusMonitor) Start()
func (m *StatusMonitor) Stop()
func (m *StatusMonitor) GetProgramStatus() (*model.OcapPerformance, error)
func ValidateHypertables(db *gorm.DB, tables map[string][]string) error
```

---

### 5. `internal/util` (~50 lines)

Small utility functions.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `trimQuotes` | ~5 | Remove surrounding quotes |
| `fixEscapeQuotes` | ~5 | Fix escaped quote characters |
| `contains` | ~8 | Check if slice contains string |

**Proposed API:**
```go
package util

func TrimQuotes(s string) string
func FixEscapeQuotes(s string) string
func Contains(slice []string, str string) bool
```

---

### 6. `internal/migration` (~300 lines) - Optional

SQLite to PostgreSQL migration utilities. Could also be a separate CLI command.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `migrateBackupsSqlite` | ~170 | Migrate SQLite backups to Postgres |
| `migrateTable` | ~40 | Generic table migration |

---

### 7. `internal/export` (~700 lines) - Optional

Recording export and data manipulation. Could be a separate CLI command.

**Functions to extract:**
| Function | Lines | Description |
|----------|-------|-------------|
| `getOcapRecording` | ~600 | Export mission to OCAP JSON format |
| `reduceMission` | ~75 | Reduce mission data size |
| `populateDemoData` | ~500 | Generate demo data (dev only) |
| `testQuery` | ~80 | Test database queries (dev only) |

---

## Execution Plan

### Phase 2A: Core Extraction (~2,400 lines)

1. **`internal/util`** - No dependencies, extract first
2. **`internal/logging`** - Depends on util
3. **`internal/handlers`** - Depends on util, model, cache, geo
4. **`internal/worker`** - Depends on handlers, queue, influx
5. **`internal/monitor`** - Depends on worker, influx

### Phase 2B: Optional Extraction (~1,000 lines)

6. **`internal/migration`** - Can be separate CLI subcommand
7. **`internal/export`** - Can be separate CLI subcommand

---

## Expected Results

### After Phase 2A

```
cmd/ocap_recorder/main.go    ~1,500 lines (down from 4,844)
internal/
  config/                       65 lines
  database/                    342 lines
  influx/                      256 lines
  logging/                     150 lines (NEW)
  handlers/                  1,500 lines (NEW)
  worker/                      500 lines (NEW)
  monitor/                     200 lines (NEW)
  util/                         50 lines (NEW)
  cache/                        93 lines
  geo/                          XX lines
  model/                       XXX lines
  queue/                       XXX lines
```

### After Phase 2B (Full)

```
cmd/ocap_recorder/main.go    ~500 lines (init, CGo exports, wiring)
internal/
  ...all packages above...
  migration/                   300 lines (NEW)
  export/                      700 lines (NEW)
```

---

## Dependencies Between New Packages

```
                    ┌─────────┐
                    │  util   │
                    └────┬────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         ┌────▼────┐ ┌───▼───┐ ┌────▼────┐
         │ logging │ │ model │ │  cache  │
         └────┬────┘ └───┬───┘ └────┬────┘
              │          │          │
              │    ┌─────▼─────┐    │
              └────► handlers  ◄────┘
                   └─────┬─────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         ┌────▼────┐ ┌───▼───┐ ┌────▼────┐
         │ worker  │ │ queue │ │ influx  │
         └────┬────┘ └───────┘ └────┬────┘
              │                     │
              └──────────┬──────────┘
                         │
                   ┌─────▼─────┐
                   │  monitor  │
                   └───────────┘
```

---

## Verification

After each package extraction:

```bash
# Build the DLL
docker run --rm -v "$(pwd):/go/work" -w /go/work x1unix/go-mingw:1.20 \
  go build -buildvcs=false -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder

# Run tests
go test ./internal/...

# Verify line count reduction
wc -l cmd/ocap_recorder/main.go
```

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking CGo exports | Keep RVExtension functions in main.go |
| Global state coupling | Use dependency injection via context structs |
| Circular dependencies | Follow dependency graph strictly |
| Performance regression | Benchmark before/after extraction |

---

## Success Criteria

- [ ] main.go reduced to <1,500 lines (Phase 2A) or <500 lines (Phase 2B)
- [ ] All tests pass
- [ ] DLL builds successfully
- [ ] No circular dependencies
- [ ] Each package has clear, single responsibility
