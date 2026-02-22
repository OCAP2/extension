# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go native DLL extension for ArmA 3 that records gameplay/mission replay data to PostgreSQL, SQLite, or in-memory JSON.

## Build Commands

```bash
# Build x64 Windows DLL
docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.24 go build -buildvcs=false -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder

# Build x64 Linux .so (uses Bullseye for glibc 2.31 compatibility)
docker run --rm -v ${PWD}:/go/work -w /go/work golang:1.24-bullseye go build -buildvcs=false -o dist/ocap_recorder_x64.so -buildmode=c-shared ./cmd/ocap_recorder
```

**Output:** `dist/ocap_recorder_x64.dll`, `dist/ocap_recorder_x64.so`

## Project Structure (Go Standard Layout)

```
/cmd/ocap_recorder/main.go   - Main application entry point
/internal/dispatcher/        - Event routing with async buffering
/internal/parser/            - Command parsing (args → core types)
/internal/worker/            - Handler registration and DB writer loop
/internal/storage/           - Storage backends (memory, postgres, sqlite, websocket)
/internal/model/             - GORM database models + converters
/internal/queue/             - Thread-safe queue implementations
/internal/cache/             - Entity caching layer
/internal/geo/               - Coordinate/geometry utilities
/pkg/a3interface/            - ArmA 3 extension interface (RVExtension exports, module path)
/pkg/core/                   - Core domain types (storage-agnostic)
Dockerfile                   - Docker build for Linux
go.mod, go.sum               - Go module dependencies
createViews.sql              - PostgreSQL materialized views
ocap_recorder.cfg.json.example - Configuration template
```

## Architecture

### DLL Entry Points (pkg/a3interface/)

CGo exports for ArmA 3 extension interface:
- `RVExtensionVersion()` - Returns version string
- `RVExtension()` - Legacy simple command handler
- `RVExtensionArgs()` - Main command dispatcher with arguments
- `RVExtensionRegisterCallback()` - Callback registration for async responses

### Command Processing (cmd/ocap_recorder/main.go)

- **Async channels:** Each command type has a buffered channel for non-blocking processing
- **Goroutines:** Dedicated goroutines consume from channels and process data
- **Thread-safe queues:** `internal/queue/queue.go` provides mutex-protected queues for DB batching

### Data Models (internal/model/model.go)

GORM models for PostgreSQL/SQLite:
- `Mission`, `World`, `Addon` - Mission metadata
- `Soldier`, `SoldierState` - Unit tracking with positions
- `Vehicle`, `VehicleState` - Vehicle tracking
- `ProjectileEvent`, `ProjectileHitsSoldier`, `ProjectileHitsVehicle` - Combat events
- `Marker`, `MarkerState` - Map marker tracking
- `PlacedObject`, `PlacedObjectEvent` - Placed object (mine/explosive) tracking

### Commands

| Command | Purpose |
|---------|---------|
| `:NEW:SOLDIER:`, `:NEW:VEHICLE:` | Register new units/vehicles |
| `:NEW:SOLDIER:STATE:`, `:NEW:VEHICLE:STATE:` | Update position/state data |
| `:PROJECTILE:`, `:KILL:` | Combat events |
| `:EVENT:`, `:CHAT:`, `:RADIO:`, `:TELEMETRY:` | General gameplay events |
| `:NEW:MARKER:`, `:NEW:MARKER:STATE:`, `:DELETE:MARKER:` | Marker operations |
| `:NEW:PLACED:`, `:PLACED:EVENT:` | Placed object (mine/explosive) lifecycle |
| `:ACE3:DEATH:`, `:ACE3:UNCONSCIOUS:` | ACE3 integration events |
| `:INIT:`, `:INIT:STORAGE:` | Initialize extension and storage |
| `:NEW:MISSION:`, `:SAVE:MISSION:` | Begin/end recording |

### Data Flow

1. Game sends commands via `RVExtensionArgs()` → dispatched to appropriate channel
2. Goroutines consume channels → populate thread-safe queues
3. DB writer goroutines batch-insert from queues → PostgreSQL/SQLite
4. On mission end → JSON export compatible with OCAP2 web frontend

## Configuration

File: `ocap_recorder.cfg.json` (placed alongside DLL)

```json
{
  "logLevel": "info",
  "logsDir": "./ocaplogs",
  "defaultTag": "TvT",
  "api": {
    "serverUrl": "http://127.0.0.1:5000/api",
    "apiKey": "secret"
  },
  "db": {
    "host": "127.0.0.1",
    "port": "5432",
    "username": "postgres",
    "password": "postgres",
    "database": "ocap"
  },
  "storage": {
    "type": "memory",
    "memory": {
      "outputDir": "./recordings",
      "compressOutput": true
    },
    "sqlite": {
      "dumpInterval": "3m"
    }
  }
}
```

Storage types: `"memory"` (JSON export), `"postgres"` (PostgreSQL), `"sqlite"` (in-memory with periodic disk dump), `"websocket"` (real-time streaming to OCAP2 web server).

## Key Dependencies

- **GORM** - ORM for PostgreSQL/SQLite
- **peterstace/simplefeatures** - Geometry/GIS support
- **log/slog** - Structured logging (stdlib)
- **spf13/viper** - Configuration management
- **gorilla/websocket** - WebSocket client for streaming storage backend
