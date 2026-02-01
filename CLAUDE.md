# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go native DLL extension for ArmA 3 that records gameplay/mission replay data to PostgreSQL (with SQLite fallback).

## Build Commands

```bash
# Pull Docker image
docker pull x1unix/go-mingw:1.24

# Build x64 Windows DLL
docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.24 go build -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder

# Build x86 Windows DLL
docker run --rm -v ${PWD}:/go/work -w /go/work -e GOARCH=386 x1unix/go-mingw:1.24 go build -o dist/ocap_recorder.dll -buildmode=c-shared ./cmd/ocap_recorder
```

**Output:** `dist/ocap_recorder_x64.dll` or `dist/ocap_recorder.dll`

## Project Structure (Go Standard Layout)

```
/cmd/ocap_recorder/main.go   - Main application entry point
/internal/model/             - GORM database models
/internal/queue/             - Thread-safe queue implementations
/internal/cache/             - Entity caching layer
/internal/geo/               - Coordinate/geometry utilities
/pkg/a3interface/            - ArmA 3 extension interface (RVExtension exports, module path)
Dockerfile                   - Docker build for Linux
go.mod, go.sum               - Go module dependencies
createViews.sql              - PostgreSQL materialized views
ocap-recorder.cfg.json.example - Configuration template
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

### Commands

| Command | Purpose |
|---------|---------|
| `:NEW:UNIT:`, `:NEW:VEH:` | Register new units/vehicles |
| `:UPDATE:UNIT:`, `:UPDATE:VEH:` | Update position/state data |
| `:EVENT:`, `:FIRED:` | Log gameplay events |
| `:MARKER:CREATE/DELETE/MOVE:` | Marker operations |
| `:START:` | Begin recording mission |
| `:SAVE:` | End recording and finalize |
| `:LOG:` | Custom log events |

### Data Flow

1. Game sends commands via `RVExtensionArgs()` → dispatched to appropriate channel
2. Goroutines consume channels → populate thread-safe queues
3. DB writer goroutines batch-insert from queues → PostgreSQL/SQLite
4. On mission end → JSON export compatible with OCAP2 web frontend

## Configuration

File: `ocap-recorder.cfg.json` (placed alongside DLL)

```json
{
  "logLevel": "info",
  "logsDir": "./OCAPLOG",
  "defaultTag": "TvT",
  "api": {
    "serverUrl": "127.0.0.1:5000",
    "apiKey": "secret"
  },
  "db": {
    "host": "127.0.0.1",
    "port": "5432",
    "username": "postgres",
    "password": "postgres",
    "database": "ocap"
  }
}
```

## Key Dependencies

- **GORM** - ORM for PostgreSQL/SQLite
- **peterstace/simplefeatures** - Geometry/GIS support
- **rs/zerolog** - Structured logging
- **spf13/viper** - Configuration management
