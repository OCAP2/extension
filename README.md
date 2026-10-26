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

Buffered handlers are gated on `:STORAGE:INIT:` — events queue in channels until the storage backend is ready.

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

### WebSocket proxy recording

To move mission processing and recording storage to the web service, use:

```json
{
  "api": {
    "serverUrl": "https://ocap.example/ocap",
    "apiKey": "same-secret",
    "uploadTimeout": "10m"
  },
  "storage": { "type": "websocket" }
}
```

Enable `streaming.enabled` on the web service and match its `secret`. The proxy
connects to the server URL plus `/api/v1/stream`, preserving any deployment prefix.
It forwards the shared protocol without a local recording database, JSON export,
or separate upload. `uploadTimeout` controls the final save acknowledgement wait.
The existing `memory` default and file-upload path remain available.

With a web service that supports resume (protocol version 2), network failures
are lossless. Every message is numbered and held in memory (up to 256 MiB) until
the web service acknowledges it. When the connection drops, the proxy keeps
recording, reconnects with backoff and jitter until it succeeds, resumes the
mission, and resends what the web service did not receive. Resent messages keep
their capture frames, so the timeline is unaffected. The proxy stops
reconnecting only when the web service rejects the secret (HTTP 401/403).

Every message is also spooled to disk until the web service saves the mission.
If the web service lost the mission (for example it restarted mid-mission),
the proxy resends the whole mission from the spool, and the web service
replaces the partial segment with the complete recording. During a long outage
memory keeps the newest messages and the rest are resent from the spool. The
spool is deleted once the mission is saved. It is kept on disk if the save is
not confirmed.

```json
"storage": {
  "type": "websocket",
  "websocket": { "bufferMB": 256, "spoolDir": "./ocapspool", "spoolMaxMB": 4096, "resendMBps": 8 }
}
```

`resendMBps` paces catch-up traffic: the backlog resent after a reconnect, a
mission resent in full from the spool, and missions recovered after a crash.
Without it, several game servers recovering at once after a web restart would
all resend at full speed and overload the web service. Live traffic is never
paced. `0` disables pacing.

`spoolDir` is relative to the Arma server's working directory, like
`memory.outputDir`; empty disables spooling. `spoolMaxMB` limits the disk used
per mission (0 = unlimited). Once the limit is reached, spooling stops for that
mission, and a later full resend is partial.

The spool also survives the game server. If it crashes mid-mission, or a save
is not confirmed, the mission stays in `spoolDir`. The next extension process
checks `spoolDir` at startup and then every minute, resends such missions to
the web service, and deletes each spool once the web service has saved its
mission:
- A mission cut short by a crash is ended on the crashed process' behalf and
  saved as `partial`, with everything recorded up to the last flush (at most
  about a second is lost).
- A mission whose save was not confirmed is saved as recorded.
- A mission whose discard was not confirmed is not resent; the web service is
  only told again to drop it.

When the addon ends a recording that does not meet its save limits (for
example one shorter than `OCAP_settings_minMissionTime`), it sends
`:MISSION:DISCARD:`, and the proxy tells the web service to drop the recording
(protocol version 3) and deletes its spool. A web service that predates
discard keeps saving such recordings as `partial`.

Game servers can share one `spoolDir`, for example several instances started
from the same directory. Each spool has an operating system file lock
(`flock` on Linux, `LockFileEx` on Windows):
- The process recording a mission holds its lock from start to end, so its
  live mission is never taken.
- A process recovering a mission holds the lock while it resends, so two
  processes never recover the same mission.
- The operating system releases the lock of a process that dies, so a
  crashed mission is recovered about 15 seconds later, with no stale claim
  to wait out.

Spools older than seven days are discarded. A web service without resume
support cannot take recovered missions: their spools stay on disk, and the
proxy asks again only every 30 minutes instead of every minute. As soon as a
mission start shows that the web service supports resume (it was upgraded),
the waiting spools are recovered.

While a mission records, the stream's state is logged every minute. The log
line lists messages sent and acknowledged, the memory buffer, messages moved
to the spool or refused, and the reconnects, resumes and restarts so far. A
disconnected stream logs at warning level
(`WebSocket stream disconnected, buffering`), so a struggling stream shows in
the logs before the mission ends.

With an older web service, a reconnect replays mission metadata and the web
service saves the received segments as partial recordings, as before. A full
buffer and a closed proxy are returned as recording errors.

Integration tests exercise the real proxy against a disposable local web server
with streaming enabled. `TestWebResumeIntegration` cuts the connection three
times mid-mission and expects one complete recording. `TestWebRestartIntegration`
also needs `OCAP_TEST_WEB_RESTART_CMD`, a shell command that restarts the web
server; it restarts the server mid-mission and expects one complete recording:

```sh
OCAP_TEST_WEB_URL=http://127.0.0.1:5000 OCAP_TEST_WEB_SECRET=same-secret \
  go test ./internal/storage/websocket -run 'TestWeb' -count=1 -v
```

Copy `ocap_recorder.cfg.json.example` to `ocap_recorder.cfg.json` alongside the DLL and edit as needed.

## Supported Commands

All commands follow a `:RESOURCE:ACTION:` naming convention. New commands must use this pattern — resource noun first, then verb/qualifier (e.g., `:SOLDIER:CREATE:`, `:EVENT:KILL:`, `:SYS:INIT:`).

### Entity Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:SOLDIER:CREATE:` | Sync | Register new unit |
| `:SOLDIER:STATE:` | 10,000 | Update unit position/state |
| `:SOLDIER:DELETE:` | 500 | Mark unit as removed (disconnect, respawn corpse) |
| `:VEHICLE:CREATE:` | Sync | Register new vehicle |
| `:VEHICLE:STATE:` | 10,000 | Update vehicle position/state |
| `:VEHICLE:DELETE:` | 500 | Mark vehicle as removed |

### Combat Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:EVENT:PROJECTILE:` | 5,000 | Projectile tracking (positions + hits) |
| `:EVENT:KILL:` | 2,000 | Kill event |

### General Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:EVENT:GENERAL:` | 1,000 | General gameplay event |
| `:EVENT:SECTOR:` | 1,000 | Sector state change (captured/contested) |
| `:EVENT:ENDMISSION:` | 100 | End-of-mission event with side and message |
| `:EVENT:CHAT:` | 1,000 | Chat message |
| `:EVENT:RADIO:` | 1,000 | Radio transmission |
| `:TELEMETRY:FRAME:` | 100 | Server telemetry (FPS, entity counts, weather, player network stats) |
| `:TIME:STATE:` | 100 | Mission time/date tracking |

### Marker Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:MARKER:CREATE:` | Sync | Create map marker (needs immediate DB ID) |
| `:MARKER:STATE:` | 1,000 | Update marker position/appearance (pos, dir, alpha, text, color, size, type, brush, shape) |
| `:MARKER:DELETE:` | 500 | Delete marker |

### Placed Object Commands

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:PLACED:CREATE:` | Sync | Register placed object (mine, explosive) |
| `:PLACED:EVENT:` | 1,000 | Placed object lifecycle event (detonated/deleted) |

### ACE3 Integration

| Command | Buffer | Purpose |
|---------|--------|---------|
| `:ACE3:DEATH:` | 1,000 | ACE3 death event |
| `:ACE3:UNCONSCIOUS:` | 1,000 | ACE3 unconscious event |

### Lifecycle Commands

| Command | Purpose |
|---------|---------|
| `:SYS:INIT:` | Initialize extension, send `:SYS:READY:` callback |
| `:STORAGE:INIT:` | Initialize storage backend, ungate buffered handlers |
| `:MISSION:START:` | Start recording mission |
| `:MISSION:SAVE:` | End recording, flush data, upload if configured |
| `:MISSION:DISCARD:` | End recording without saving it (`memory`, `websocket`); answers with a `:MISSION:DISCARDED:` callback |
| `:MISSION:FOCUS_START:` | Set playback focus start frame |
| `:MISSION:FOCUS_END:` | Set playback focus end frame |
| `:SYS:VERSION:` | Get extension version |
