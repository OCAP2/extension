// internal/api/client_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	c := New("http://localhost:5000", "secret123")

	require.NotNil(t, c)
	assert.Equal(t, "http://localhost:5000", c.baseURL)
	assert.Equal(t, "secret123", c.apiKey)
	assert.NotNil(t, c.httpClient)
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("http://localhost:5000/", "secret")
	assert.Equal(t, "http://localhost:5000", c.baseURL)
}

func TestHealthcheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/healthcheck", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "")
	err := c.Healthcheck()
	assert.NoError(t, err)
}

func TestHealthcheck_ServerDown(t *testing.T) {
	c := New("http://localhost:59999", "")
	err := c.Healthcheck()
	assert.Error(t, err)
}

func TestHealthcheck_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := New(server.URL, "")
	err := c.Healthcheck()
	assert.Error(t, err)
}

func TestUpload_Success(t *testing.T) {
	var receivedSecret, receivedFilename, receivedWorldName string
	var receivedMissionName, receivedTag string
	var receivedDuration string
	var receivedFileContent []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/operations/add", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseMultipartForm(10 << 20)
		require.NoError(t, err)

		receivedSecret = r.FormValue("secret")
		receivedFilename = r.FormValue("filename")
		receivedWorldName = r.FormValue("worldName")
		receivedMissionName = r.FormValue("missionName")
		receivedDuration = r.FormValue("missionDuration")
		receivedTag = r.FormValue("tag")

		file, _, err := r.FormFile("file")
		require.NoError(t, err)
		defer func() { _ = file.Close() }()

		receivedFileContent = make([]byte, 1024)
		n, _ := file.Read(receivedFileContent)
		receivedFileContent = receivedFileContent[:n]

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	testFile := tmpDir + "/test_mission.json.gz"
	err := writeTestFile(testFile, []byte("test content"))
	require.NoError(t, err)

	c := New(server.URL, "mysecret")
	meta := core.UploadMetadata{
		WorldName:       "Altis",
		MissionName:     "Test Mission",
		MissionDuration: 3600.5,
		Tag:             "TvT",
	}

	err = c.Upload(testFile, meta)
	require.NoError(t, err)

	assert.Equal(t, "mysecret", receivedSecret)
	assert.Equal(t, "test_mission.json.gz", receivedFilename)
	assert.Equal(t, "Altis", receivedWorldName)
	assert.Equal(t, "Test Mission", receivedMissionName)
	assert.Equal(t, "3600.500000", receivedDuration)
	assert.Equal(t, "TvT", receivedTag)
	assert.Equal(t, "test content", string(receivedFileContent))
}

func TestUpload_FileNotFound(t *testing.T) {
	c := New("http://localhost:5000", "secret")
	err := c.Upload("/nonexistent/file.json.gz", core.UploadMetadata{})
	assert.Error(t, err)
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
	err := c.Upload(testFile, core.UploadMetadata{})
	assert.Error(t, err)
}

func TestUpload_ServerDown(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := tmpDir + "/test.json.gz"
	_ = writeTestFile(testFile, []byte("content"))

	// Server URL that is unreachable
	c := New("http://localhost:59999", "secret")
	err := c.Upload(testFile, core.UploadMetadata{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upload request failed")
}

func writeTestFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0644)
}
