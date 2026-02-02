# OCAP Recorder (Go)

![Coverage](https://raw.githubusercontent.com/OCAP2/extension/badges/.badges/main/coverage.svg)

## About

A Go implementation of an ArmA 3 native extension that records gameplay to PostgreSQL (with SQLite fallback). Captures unit positions, combat events, markers, and more for mission replay and analytics.

## Architecture

### Overview

```
ArmA 3 Game
    ↓ callExtension("ocap_recorder", [":COMMAND:", args])
RVExtensionArgs() [CGo boundary]
    ↓
Dispatcher.Dispatch(Event)
    ↓
    ├─ [Buffered] → Channel → Goroutine → Handler
    └─ [Sync]     → Handler directly
           ↓
Handler (parse args, validate, create model)
    ↓
    ├─ Storage Backend (if configured)
    └─ Queue (traditional) → batch insert every 2s
           ↓
DB Writer Loop
    ↓
PostgreSQL / SQLite
```

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
├── worker/                  Handler registration and DB writer loop
├── handlers/                Business logic for parsing and validation
├── queue/                   Thread-safe queues for batch writes
├── cache/                   Entity lookup caching (OcapID → model)
├── model/                   GORM database models
├── storage/                 Optional alternative storage backend
└── geo/                     Coordinate/geometry utilities
```

### Design Principles

1. **Low latency**: Async buffered handlers don't block ArmA's game loop
2. **High throughput**: Batch writes every 2 seconds instead of per-event
3. **Entity caching**: Sync entity creation → cache → async state updates use cached FK
4. **Graceful fallback**: PostgreSQL → SQLite if connection fails
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
docker build -t indifox926/build-a3go:linux-so -f ./Dockerfile .

docker run --rm -v ${PWD}:/app -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=1 -e CC=gcc \
  indifox926/build-a3go:linux-so go build -o dist/ocap_recorder_x64.so -linkshared ./cmd/ocap_recorder
```

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
| `:FIRED:` | 10,000 | Weapon fire event |
| `:PROJECTILE:` | 5,000 | Projectile tracking |
| `:HIT:` | 2,000 | Damage event |
| `:KILL:` | 2,000 | Kill event |

### General Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:EVENT:` | 1,000 | General gameplay event |
| `:CHAT:` | 1,000 | Chat message |
| `:RADIO:` | 1,000 | Radio transmission |
| `:FPS:` | 1,000 | Server FPS sample |

### Marker Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:MARKER:CREATE:` | 500 | Create map marker |
| `:MARKER:MOVE:` | 1,000 | Update marker position |
| `:MARKER:DELETE:` | 500 | Delete marker |

### ACE3 Integration

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:ACE3:DEATH:` | 1,000 | ACE3 death event |
| `:ACE3:UNCONSCIOUS:` | 1,000 | ACE3 unconscious event |

### Lifecycle Commands

| Command | Purpose |
|---------|---------|
| `:INIT:` | Initialize extension |
| `:INIT:DB:` | Connect to database |
| `:NEW:MISSION:` | Start recording mission |
| `:SAVE:` | End recording, flush data |
| `:VERSION:` | Get extension version |
