# API Upload Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upload mission JSON to OCAP web frontend after export completes.

**Architecture:** Local-first approach - write JSON to disk, then upload. New `internal/api` package for HTTP client. Optional `Uploadable` interface allows memory backend to provide upload metadata without affecting database backend.

**Tech Stack:** Go standard library `net/http`, `mime/multipart`

---

### Task 1: Add Uploadable Interface to storage.go

**Files:**
- Modify: `internal/storage/storage.go`

**Step 1: Write the failing test**

Create test file to verify interface compliance:

```go
// internal/storage/storage_test.go
package storage_test

import (
	"testing"

	"github.com/OCAP2/extension/v5/internal/storage"
)

func TestUploadMetadataFields(t *testing.T) {
	meta := storage.UploadMetadata{
		WorldName:       "Altis",
		MissionName:     "Test Mission",
		MissionDuration: 3600.5,
		Tag:             "TvT",
	}

	if meta.WorldName != "Altis" {
		t.Errorf("expected WorldName=Altis, got %s", meta.WorldName)
	}
	if meta.MissionName != "Test Mission" {
		t.Errorf("expected MissionName=Test Mission, got %s", meta.MissionName)
	}
	if meta.MissionDuration != 3600.5 {
		t.Errorf("expected MissionDuration=3600.5, got %f", meta.MissionDuration)
	}
	if meta.Tag != "TvT" {
		t.Errorf("expected Tag=TvT, got %s", meta.Tag)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/... -v -run TestUploadMetadataFields`
Expected: FAIL - `storage.UploadMetadata` undefined

**Step 3: Write minimal implementation**

Add to `internal/storage/storage.go` after the Backend interface:

```go
// UploadMetadata contains mission information needed for upload.
type UploadMetadata struct {
	WorldName       string
	MissionName     string
	MissionDuration float64
	Tag             string
}

// Uploadable is an optional interface for storage backends that produce
// files suitable for upload to the OCAP web frontend.
type Uploadable interface {
	GetExportedFilePath() string
	GetExportMetadata() UploadMetadata
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage/... -v -run TestUploadMetadataFields`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/storage.go internal/storage/storage_test.go
git commit -m "$(cat <<'EOF'
feat(storage): add Uploadable interface and UploadMetadata type

Optional interface for backends that produce uploadable files.
EOF
)"
```

---

### Task 2: Implement Uploadable in Memory Backend

**Files:**
- Modify: `internal/storage/memory/memory.go`
- Modify: `internal/storage/memory/export.go`
- Modify: `internal/storage/memory/memory_test.go`

**Step 1: Write the failing test**

Add to `internal/storage/memory/memory_test.go`:

```go
// Verify Backend implements storage.Uploadable interface
var _ storage.Uploadable = (*Backend)(nil)

func TestGetExportedFilePath(t *testing.T) {
	b := New(config.MemoryConfig{
		OutputDir:      t.TempDir(),
		CompressOutput: true,
	})

	// Before export, should return empty
	if path := b.GetExportedFilePath(); path != "" {
		t.Errorf("expected empty path before export, got %s", path)
	}
}

func TestGetExportMetadata(t *testing.T) {
	b := New(config.MemoryConfig{})

	mission := &core.Mission{
		MissionName:  "Test Mission",
		CaptureDelay: 1.0,
		Tag:          "TvT",
	}
	world := &core.World{
		WorldName: "Altis",
	}

	_ = b.StartMission(mission, world)

	// Add some frames
	s := &core.Soldier{OcapID: 1}
	_ = b.AddSoldier(s)
	_ = b.RecordSoldierState(&core.SoldierState{
		SoldierID:    s.ID,
		CaptureFrame: 100,
	})

	meta := b.GetExportMetadata()

	if meta.WorldName != "Altis" {
		t.Errorf("expected WorldName=Altis, got %s", meta.WorldName)
	}
	if meta.MissionName != "Test Mission" {
		t.Errorf("expected MissionName=Test Mission, got %s", meta.MissionName)
	}
	if meta.Tag != "TvT" {
		t.Errorf("expected Tag=TvT, got %s", meta.Tag)
	}
	// Duration = endFrame * captureDelay / 1000 = 100 * 1.0 / 1000 = 0.1
	if meta.MissionDuration != 0.1 {
		t.Errorf("expected MissionDuration=0.1, got %f", meta.MissionDuration)
	}
}

func TestStartMissionResetsExportPath(t *testing.T) {
	b := New(config.MemoryConfig{
		OutputDir:      t.TempDir(),
		CompressOutput: true,
	})

	mission := &core.Mission{
		MissionName: "First",
		StartTime:   time.Now(),
	}
	world := &core.World{WorldName: "Altis"}

	_ = b.StartMission(mission, world)
	_ = b.EndMission()

	firstPath := b.GetExportedFilePath()
	if firstPath == "" {
		t.Fatal("expected non-empty path after export")
	}

	// Start new mission - should reset path
	_ = b.StartMission(&core.Mission{MissionName: "Second", StartTime: time.Now()}, world)

	if path := b.GetExportedFilePath(); path != "" {
		t.Errorf("expected empty path after StartMission, got %s", path)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/memory/... -v -run "TestGetExportedFilePath|TestGetExportMetadata|TestStartMissionResetsExportPath"`
Expected: FAIL - methods not defined

**Step 3: Write minimal implementation**

Add field to Backend struct in `internal/storage/memory/memory.go`:

```go
type Backend struct {
	cfg     config.MemoryConfig
	mission *core.Mission
	world   *core.World

	lastExportPath string // Add this field

	soldiers map[uint16]*SoldierRecord
	// ... rest unchanged
}
```

Reset in StartMission (add after `b.idCounter = 0`):

```go
	b.lastExportPath = ""
```

Add methods at end of `internal/storage/memory/memory.go`:

```go
// GetExportedFilePath returns the path to the last exported file.
func (b *Backend) GetExportedFilePath() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastExportPath
}

// GetExportMetadata returns metadata about the last export.
func (b *Backend) GetExportMetadata() storage.UploadMetadata {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var endFrame uint
	for _, record := range b.soldiers {
		for _, state := range record.States {
			if state.CaptureFrame > endFrame {
				endFrame = state.CaptureFrame
			}
		}
	}
	for _, record := range b.vehicles {
		for _, state := range record.States {
			if state.CaptureFrame > endFrame {
				endFrame = state.CaptureFrame
			}
		}
	}

	duration := float64(endFrame) * float64(b.mission.CaptureDelay) / 1000.0

	return storage.UploadMetadata{
		WorldName:       b.world.WorldName,
		MissionName:     b.mission.MissionName,
		MissionDuration: duration,
		Tag:             b.mission.Tag,
	}
}
```

Set lastExportPath in `internal/storage/memory/export.go`, at end of exportJSON before return:

```go
func (b *Backend) exportJSON() error {
	// ... existing code ...

	// Write file
	if b.cfg.CompressOutput {
		if err := b.writeGzipJSON(outputPath, export); err != nil {
			return err
		}
	} else {
		if err := b.writeJSON(outputPath, export); err != nil {
			return err
		}
	}

	b.lastExportPath = outputPath
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage/memory/... -v -run "TestGetExportedFilePath|TestGetExportMetadata|TestStartMissionResetsExportPath"`
Expected: PASS

**Step 5: Run all memory tests**

Run: `go test ./internal/storage/memory/... -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/storage/memory/memory.go internal/storage/memory/export.go internal/storage/memory/memory_test.go
git commit -m "$(cat <<'EOF'
feat(memory): implement Uploadable interface

- Add lastExportPath field, set after successful export
- GetExportedFilePath returns path to exported file
- GetExportMetadata returns mission info for upload
- StartMission resets lastExportPath
EOF
)"
```

---

### Task 3: Create API Client Package

**Files:**
- Create: `internal/api/client.go`
- Create: `internal/api/client_test.go`

**Step 1: Write the failing test**

```go
// internal/api/client_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OCAP2/extension/v5/internal/storage"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:5000", "secret123")

	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.baseURL != "http://localhost:5000" {
		t.Errorf("expected baseURL=http://localhost:5000, got %s", c.baseURL)
	}
	if c.apiKey != "secret123" {
		t.Errorf("expected apiKey=secret123, got %s", c.apiKey)
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("http://localhost:5000/", "secret")
	if c.baseURL != "http://localhost:5000" {
		t.Errorf("expected trailing slash trimmed, got %s", c.baseURL)
	}
}

func TestHealthcheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthcheck" {
			t.Errorf("expected path /healthcheck, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "")
	err := c.Healthcheck()
	if err != nil {
		t.Errorf("Healthcheck failed: %v", err)
	}
}

func TestHealthcheck_ServerDown(t *testing.T) {
	c := New("http://localhost:59999", "") // unlikely to be listening
	err := c.Healthcheck()
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestHealthcheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := New(server.URL, "")
	err := c.Healthcheck()
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestUpload_Success(t *testing.T) {
	var receivedSecret, receivedFilename, receivedWorldName string
	var receivedMissionName, receivedTag string
	var receivedDuration string
	var receivedFileContent []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/operations/add" {
			t.Errorf("expected path /api/v1/operations/add, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		receivedSecret = r.FormValue("secret")
		receivedFilename = r.FormValue("filename")
		receivedWorldName = r.FormValue("worldName")
		receivedMissionName = r.FormValue("missionName")
		receivedDuration = r.FormValue("missionDuration")
		receivedTag = r.FormValue("tag")

		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("failed to get file: %v", err)
		}
		defer file.Close()

		receivedFileContent = make([]byte, 1024)
		n, _ := file.Read(receivedFileContent)
		receivedFileContent = receivedFileContent[:n]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create temp file
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test_mission.json.gz"
	if err := writeTestFile(testFile, []byte("test content")); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	c := New(server.URL, "mysecret")
	meta := storage.UploadMetadata{
		WorldName:       "Altis",
		MissionName:     "Test Mission",
		MissionDuration: 3600.5,
		Tag:             "TvT",
	}

	err := c.Upload(testFile, meta)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if receivedSecret != "mysecret" {
		t.Errorf("expected secret=mysecret, got %s", receivedSecret)
	}
	if receivedFilename != "test_mission.json.gz" {
		t.Errorf("expected filename=test_mission.json.gz, got %s", receivedFilename)
	}
	if receivedWorldName != "Altis" {
		t.Errorf("expected worldName=Altis, got %s", receivedWorldName)
	}
	if receivedMissionName != "Test Mission" {
		t.Errorf("expected missionName=Test Mission, got %s", receivedMissionName)
	}
	if receivedDuration != "3600.500000" {
		t.Errorf("expected missionDuration=3600.500000, got %s", receivedDuration)
	}
	if receivedTag != "TvT" {
		t.Errorf("expected tag=TvT, got %s", receivedTag)
	}
	if string(receivedFileContent) != "test content" {
		t.Errorf("expected file content 'test content', got '%s'", string(receivedFileContent))
	}
}

func TestUpload_FileNotFound(t *testing.T) {
	c := New("http://localhost:5000", "secret")
	err := c.Upload("/nonexistent/file.json.gz", storage.UploadMetadata{})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestUpload_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.json.gz"
	_ = writeTestFile(testFile, []byte("content"))

	c := New(server.URL, "wrong-secret")
	err := c.Upload(testFile, storage.UploadMetadata{})
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func writeTestFile(path string, content []byte) error {
	return writeFile(path, content)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api/... -v`
Expected: FAIL - package doesn't exist

**Step 3: Write minimal implementation**

```go
// internal/api/client.go
package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/internal/storage"
)

// Client handles communication with the OCAP web frontend.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new API client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Healthcheck checks if the OCAP web frontend is reachable.
func (c *Client) Healthcheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/healthcheck")
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", resp.StatusCode)
	}
	return nil
}

// Upload sends a gzipped JSON mission file to the OCAP web frontend.
func (c *Client) Upload(filePath string, meta storage.UploadMetadata) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write form fields and file in goroutine
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		defer writer.Close()

		// Form fields
		_ = writer.WriteField("secret", c.apiKey)
		_ = writer.WriteField("filename", filepath.Base(filePath))
		_ = writer.WriteField("worldName", meta.WorldName)
		_ = writer.WriteField("missionName", meta.MissionName)
		_ = writer.WriteField("missionDuration", fmt.Sprintf("%f", meta.MissionDuration))
		_ = writer.WriteField("tag", meta.Tag)

		// File
		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			errCh <- fmt.Errorf("failed to create form file: %w", err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			errCh <- fmt.Errorf("failed to copy file: %w", err)
			return
		}
		errCh <- nil
	}()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/operations/add", pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check goroutine error
	if writeErr := <-errCh; writeErr != nil {
		return writeErr
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload returned status %d", resp.StatusCode)
	}
	return nil
}

// writeFile is a helper for tests
func writeFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0644)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/api/... -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/api/client.go internal/api/client_test.go
git commit -m "$(cat <<'EOF'
feat(api): add API client for OCAP web frontend

- New() creates client with base URL and API key
- Healthcheck() checks if frontend is reachable
- Upload() sends gzipped JSON via multipart form
EOF
)"
```

---

### Task 4: Integrate API Client in main.go

**Files:**
- Modify: `cmd/ocap_recorder/main.go`

**Step 1: Add import and package variable**

Add import:
```go
"github.com/OCAP2/extension/v5/internal/api"
```

Add after `storageBackend storage.Backend`:
```go
	// API client for OCAP web frontend
	apiClient *api.Client
```

**Step 2: Create initAPIClient function**

Add after `checkServerStatus()` function:
```go
func initAPIClient() {
	serverURL := viper.GetString("api.serverUrl")
	if serverURL == "" {
		Logger.Info("API server URL not configured, upload disabled")
		return
	}

	apiKey := viper.GetString("api.apiKey")
	apiClient = api.New(serverURL, apiKey)

	if err := apiClient.Healthcheck(); err != nil {
		Logger.Info("OCAP Frontend is offline", "error", err)
	} else {
		Logger.Info("OCAP Frontend is online")
	}
}
```

**Step 3: Replace checkServerStatus call with initAPIClient**

In `init()` goroutine (around line 286), change:
```go
go func() {
	startGoroutines()

	// log frontend status
	checkServerStatus()
}()
```
to:
```go
go func() {
	startGoroutines()

	// Initialize API client and check frontend status
	initAPIClient()
}()
```

**Step 4: Delete checkServerStatus function**

Remove the function (lines 353-364):
```go
func checkServerStatus() {
	// ... remove entirely
}
```

**Step 5: Update :SAVE: handler**

Replace the existing :SAVE: handler with:
```go
d.Register(":SAVE:", func(e dispatcher.Event) (any, error) {
	Logger.Info("Received :SAVE: command, ending mission recording")
	if storageBackend != nil {
		if err := storageBackend.EndMission(); err != nil {
			Logger.Error("Failed to end mission in storage backend", "error", err)
			return nil, err
		}
		Logger.Info("Mission recording saved to storage backend")

		// Upload if backend supports it and API client is configured
		if u, ok := storageBackend.(storage.Uploadable); ok && apiClient != nil {
			if path := u.GetExportedFilePath(); path != "" {
				meta := u.GetExportMetadata()
				if err := apiClient.Upload(path, meta); err != nil {
					Logger.Error("Failed to upload to OCAP web", "error", err, "path", path)
					// Don't return error - file is saved locally
				} else {
					Logger.Info("Mission uploaded to OCAP web", "path", path)
				}
			}
		}
	}
	// Flush OTel data if provider is available
	if OTelProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := OTelProvider.Flush(ctx); err != nil {
			Logger.Warn("Failed to flush OTel data", "error", err)
		}
	}
	return "ok", nil
})
```

**Step 6: Verify build**

Run: `go build ./cmd/ocap_recorder/...`
Expected: Build succeeds

**Step 7: Run all tests**

Run: `go test ./... -v`
Expected: All PASS

**Step 8: Commit**

```bash
git add cmd/ocap_recorder/main.go
git commit -m "$(cat <<'EOF'
feat: integrate API upload in :SAVE: handler

- Add apiClient package variable
- initAPIClient() replaces checkServerStatus()
- :SAVE: handler uploads if backend is Uploadable
- Upload failure is logged but doesn't fail command
EOF
)"
```

---

### Task 5: Final Verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

**Step 2: Build DLL**

Run:
```bash
docker run --rm -v ${PWD}:/go/work -w /go/work x1unix/go-mingw:1.24 go build -o dist/ocap_recorder_x64.dll -buildmode=c-shared ./cmd/ocap_recorder
```
Expected: Build succeeds, produces `dist/ocap_recorder_x64.dll`

**Step 3: Commit any remaining changes**

If any uncommitted changes remain, commit them.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add Uploadable interface | storage.go |
| 2 | Implement Uploadable in memory backend | memory.go, export.go |
| 3 | Create API client package | api/client.go |
| 4 | Integrate in main.go | main.go |
| 5 | Final verification | - |
