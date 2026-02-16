# OCAP Recorder (Go)

![Coverage](https://raw.githubusercontent.com/OCAP2/extension/badges/.badges/main/coverage.svg)

## About

A Go implementation of an ArmA 3 native extension that records gameplay to PostgreSQL, SQLite, or in-memory JSON. Captures unit positions, combat events, markers, and more for mission replay and analytics.

## Architecture

### Overview

```
ArmA 3 Game
    ↓ callExtension("ocap_recorder", [":COMMAND:", args])
RVExtensionArgs() [CGo boundary]
    ↓
Dispatcher.Dispatch(Event)
    ↓
    ├─ [Sync]     → Handler directly
    └─ [Buffered] → Channel → Goroutine → Handler
                                              ↓
                                   Parser (args → core types)
                                              ↓
                                   EntityCache (validate/enrich)
                                              ↓
                                   Storage Backend
                                     ├─ Memory → in-memory append → JSON export on save
                                     ├─ Postgres → Queue → DB writer (batch insert every 2s) → PostgreSQL
                                     └─ SQLite → Queue → DB writer (batch insert every 2s) → SQLite
```

Buffered handlers are gated on `:INIT:STORAGE:` — events queue in channels until the storage backend is ready.

### DLL Entry Points

The extension exposes CGo-exported functions that ArmA 3 calls:

| Function | Purpose |
|----------|---------|
| `RVExtensionArgs()` | Main entry point; receives command and arguments |
| `RVExtension()` | Legacy simple command handler |
| `RVExtensionVersion()` | Returns version string |

### Event Dispatcher

The dispatcher routes commands to handlers with optional buffering:

- **Sync handlers**: Execute immediately (entity creation)
- **Buffered handlers**: Queue events in channels for async processing (high-volume state updates)
- **Metrics**: OpenTelemetry integration for queue sizes, events processed/dropped

### Project Structure

```
cmd/ocap_recorder/main.go    Entry point, initialization, lifecycle commands
pkg/a3interface/             CGo exports (RVExtension*)
internal/
├── dispatcher/              Event routing with async buffering
├── parser/                  Command parsing (args → core types)
├── worker/                  Handler registration and DB writer loop
├── queue/                   Thread-safe queues for batch writes
├── cache/                   Entity lookup caching (ObjectID → model)
├── model/                   Database models + converters
├── storage/                 Storage backends (memory, postgres, sqlite)
└── geo/                     Coordinate/geometry utilities
```

### Design Principles

1. **Low latency**: Async buffered handlers don't block ArmA's game loop
2. **High throughput**: Batch writes every 2 seconds instead of per-event
3. **Entity caching**: Sync entity creation → cache → async state updates use cached FK
4. **Pluggable storage**: Memory (JSON export), PostgreSQL, or SQLite (in-memory with periodic disk dump)
5. **Observability**: OpenTelemetry metrics and structured logging (slog)

## Building

Requires Docker with Linux containers.

### Windows DLL

```bash
docker pull x1unix/go-mingw:1.24

docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.24 \
  go build -buildvcs=false -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder
```

### Linux .so

```bash
docker run --rm -v ${PWD}:/go/work -w /go/work golang:1.24-bullseye \
  go build -buildvcs=false -o dist/ocap_recorder_x64.so -buildmode=c-shared ./cmd/ocap_recorder
```

Uses Debian Bullseye (glibc 2.31) for broad compatibility with Linux game servers.

## Configuration

Copy `ocap_recorder.cfg.json.example` to `ocap_recorder.cfg.json` alongside the DLL and edit as needed.

## Supported Commands

### Entity Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:NEW:SOLDIER:` | Sync | Register new unit |
| `:NEW:VEHICLE:` | Sync | Register new vehicle |
| `:NEW:SOLDIER:STATE:` | 10,000 | Update unit position/state |
| `:NEW:VEHICLE:STATE:` | 10,000 | Update vehicle position/state |

### Combat Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:PROJECTILE:` | 5,000 | Projectile tracking (positions + hits) |
| `:KILL:` | 2,000 | Kill event |

### General Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:EVENT:` | 1,000 | General gameplay event |
| `:CHAT:` | 1,000 | Chat message |
| `:RADIO:` | 1,000 | Radio transmission |
| `:FPS:` | 1,000 | Server FPS sample |
| `:NEW:TIME:STATE:` | 100 | Mission time/date tracking |

### Marker Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:NEW:MARKER:` | Sync | Create map marker (needs immediate DB ID) |
| `:NEW:MARKER:STATE:` | 1,000 | Update marker position/appearance |
| `:DELETE:MARKER:` | 500 | Delete marker |

### ACE3 Integration

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:ACE3:DEATH:` | 1,000 | ACE3 death event |
| `:ACE3:UNCONSCIOUS:` | 1,000 | ACE3 unconscious event |

### Lifecycle Commands

| Command | Purpose |
|---------|---------|
| `:INIT:` | Initialize extension, send `:EXT:READY:` callback |
| `:INIT:STORAGE:` | Initialize storage backend, ungate buffered handlers |
| `:NEW:MISSION:` | Start recording mission |
| `:SAVE:MISSION:` | End recording, flush data, upload if configured |
| `:VERSION:` | Get extension version |
