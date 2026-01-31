# OCAP Recorder (Go)

## About

This is a Go implementation of an Arma 3 extension that allows for recording of gameplay to a Postgres database (with local SQLite backup capabilities). It offers extended data capture due to the storage medium for the purposes of playback resolution and analytics.

It includes the ability to send its own performance metrics to InfluxDB for monitoring and alerting.

## Building using Docker

You will need Docker Engine installed and running. This can be done on Windows or on Linux. However, you will need to use Linux containers if you're on Windows (specified in Docker Desktop settings).

### Compiling for Windows

```bash
docker pull x1unix/go-mingw:1.20

# Compile x64 Windows DLL
docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.20 go build -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder

# Compile x86 Windows DLL
docker run --rm -v ${PWD}:/go/work -w /go/work -e GOARCH=386 x1unix/go-mingw:1.20 go build -o dist/ocap_recorder.dll -buildmode=c-shared ./cmd/ocap_recorder

# Compile x64 Windows EXE
docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.20 go build -o dist/ocap_recorder_x64.exe ./cmd/ocap_recorder
```

### Compiling for Linux

```bash
docker build -t indifox926/build-a3go:linux-so -f ./build/Dockerfile.build ./cmd

# Compile x64 Linux .so
docker run --rm -v ${PWD}:/app -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=1 -e CC=gcc indifox926/build-a3go:linux-so go build -o dist/ocap_recorder_x64.so -linkshared ./cmd/ocap_recorder

# Compile x86 Linux .so
docker run --rm -v ${PWD}:/app -e GOOS=linux -e GOARCH=386 -e CGO_ENABLED=1 -e CC=gcc indifox926/build-a3go:linux-so go build -o dist/ocap_recorder.so -linkshared ./cmd/ocap_recorder
```

## Supported Commands

### Marker Commands
- `:MARKER:CREATE:` - Create a new map marker
- `:MARKER:MOVE:` - Update marker position
- `:MARKER:DELETE:` - Delete a marker

### Data Format
Markers are stored with full history of position changes for replay.
