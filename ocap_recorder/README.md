# ocap_recorder (Golang)

## About

This is a Golang implementation of an Arma 3 extension that allows for recording of gameplay to a Postgres database (with local SQLite backup capabilities). It offers extended data capture due to the storage medium for the purposes of playback resolution and analytics.

It includes the ability to send its own performance metrics to InfluxDB for monitoring and alerting.

## Building with Go

### COMPILING FOR WINDOWS x64 ON WINDOWS x64

```ps1
$ENV:GOARCH = "amd64"
$ENV:CGO_ENABLED = 1
$ENV:GOOS = "windows"
go build -o dist/ocap_recorder_x64.dll -buildmode=c-shared .
go build -o dist/ocap_recorder_x64.exe -buildmode=exe .

# You can view exports of the created dll using dumpbin, which is included with Visual Studio installations
"C:\Program Files (x86)\Microsoft Visual Studio\2019\Community\VC\Tools\MSVC\14.29.30037\bin\Hostx86\x86\dumpbin.exe" /exports ocap_recorder_x64.dll
```

## Building using Docker

You will need Docker Engine installed and running. This can be done on Windows or on Linux. However, you will need to use Linux containers if you're on Windows (specified in Docker Desktop settings).

The below assumes you're running the commands from the `ocap_recorder` directory.

### COMPILING FOR WINDOWS

```bash
docker pull x1unix/go-mingw:1.20

# Compile x64 Windows DLL
docker run --rm -it -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.20 go build -o dist/ocap_recorder_x64.dll -buildmode=c-shared .

# Compile x86 Windows DLL
docker run --rm -it -v ${PWD}:/go/work -w /go/work -e GOARCH=386 x1unix/go-mingw:1.20 go build -o dist/ocap_recorder.dll -buildmode=c-shared .
```

### COMPILING FOR LINUX

```bash
docker build -t indifox926/build-a3go:linux-so -f Dockerfile.build .

# Compile x64 Linux .so
docker run --rm -it -v ${PWD}:/app -e GOOS=linux -e GOARCH=amd64 -e CGO_ENABLED=1 -e CC=gcc indifox926/build-a3go:linux-so go build -o dist/ocap_recorder_x64.so -linkshared .

# Compile x86 Linux .so
docker run --rm -it -v ${PWD}:/app -e GOOS=linux -e GOARCH=386 -e CGO_ENABLED=1 -e CC=gcc indifox926/build-a3go:linux-so go build -o dist/ocap_recorder.so -linkshared .
```
