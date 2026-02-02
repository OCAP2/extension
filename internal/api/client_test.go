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
