# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

C++ native DLL extension for ArmA 3 that records gameplay/mission replay data and transmits it to an OCAP2 (Operational Combat Analysis Platform) backend server.

## Build Commands

```bash
# Configure (Windows with Visual Studio 2019)
cmake -B build -S OcapReplaySaver2 -G "Visual Studio 16 2019" -A x64   # 64-bit
cmake -B build -S OcapReplaySaver2 -G "Visual Studio 16 2019" -A Win32 # 32-bit

# Build
cmake --build build --config Release

# Install (optional)
cmake --install build --prefix <destination>
```

**Dependencies:** CURL, zlib (must be installed before CMake configuration)

**Output:** `OcapReplaySaver2.dll` (32-bit) or `OcapReplaySaver2_x64.dll` (64-bit)

## Architecture

### DLL Entry Points (OcapReplaySaver2.h)

Three exported functions for ArmA 3 extension interface:
- `RVExtensionVersion()` - Returns version string
- `RVExtension()` - Legacy simple command handler
- `RVExtensionArgs()` - Main command dispatcher with arguments

### Command Processing (OcapReplaySaver2.cpp)

- **Threading:** Background worker thread processes commands from a thread-safe queue
- **Command dispatch:** `dll_commands` unordered_map maps command strings to handler functions
- **State:** `is_writing` atomic flag controls recording state

### Commands

| Command | Purpose |
|---------|---------|
| `:NEW:UNIT:`, `:NEW:VEH:` | Register new units/vehicles |
| `:UPDATE:UNIT:`, `:UPDATE:VEH:` | Update position data |
| `:EVENT:`, `:FIRED:` | Log gameplay events |
| `:MARKER:CREATE/DELETE/MOVE:` | Marker operations |
| `:START:` | Begin recording |
| `:SAVE:` | Save and transmit to backend |
| `:CLEAR:` | Clear JSON buffer |
| `:TIME:` | Set recording time |
| `:VERSION:`, `:SET:VERSION:` | Version management |

### Data Flow

1. Game sends commands via `RVExtensionArgs()` → queued in thread-safe queue
2. Background thread processes commands → builds JSON data structure
3. On `:SAVE:` → gzip compress → HTTP POST to backend API
4. Failed transmissions saved locally in `OCAPLOG/` for retry

## Configuration

File: `OcapReplaySaver2.cfg.json` (placed alongside DLL)

```json
{
  "newUrl": "http://127.0.0.1/api/v1/operations/add",
  "newUrlRequestSecret": "pwd1234",
  "httpRequestTimeout": 120,
  "traceLog": 0,
  "logsDir": "./OCAPLOG",
  "logAndTmpPrefix": "ocap-",
  "newServerGameType": "TvT"
}
```

Set `traceLog: 1` to enable verbose logging.

## Key Libraries

- **nlohmann/json** (json.hpp) - JSON serialization
- **easylogging++** - Logging framework
- **CURL** - HTTP requests
- **zlib** - gzip compression
