// internal/api/client.go
package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/pkg/core"
)

// PathPrefix is the API path prefix appended to the base server URL.
const PathPrefix = "/api"

// DefaultUploadTimeout is the default upload timeout when the caller
// does not provide one. It covers the full request (including body
// streaming), so it must be generous enough for tens-of-MB uploads
// across a slow reverse proxy.
const DefaultUploadTimeout = 10 * time.Minute

// ClientConfig configures the API client. All fields are optional —
// zero values resolve to sensible defaults.
type ClientConfig struct {
	// UploadTimeout is the maximum duration for the total upload request
	// (including streaming the body). Defaults to DefaultUploadTimeout.
	UploadTimeout time.Duration
}

// Client handles communication with the OCAP web frontend.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new API client with default configuration.
func New(baseURL, apiKey string) *Client {
	return NewWithConfig(baseURL, apiKey, ClientConfig{})
}

// NewWithConfig creates a new API client with custom configuration.
func NewWithConfig(baseURL, apiKey string, cfg ClientConfig) *Client {
	timeout := cfg.UploadTimeout
	if timeout <= 0 {
		timeout = DefaultUploadTimeout
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/") + PathPrefix,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// Healthcheck checks if the OCAP web frontend is reachable.
func (c *Client) Healthcheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/healthcheck")
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", resp.StatusCode)
	}
	return nil
}

// Upload sends a gzipped JSON mission file to the OCAP web frontend.
func (c *Client) Upload(filePath string, meta core.UploadMetadata) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Create multipart form
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write form fields and file in goroutine
	errCh := make(chan error, 1)
	go func() {
		defer func() { _ = pw.Close() }()
		defer func() { _ = writer.Close() }()

		// Form fields
		_ = writer.WriteField("secret", c.apiKey)
		_ = writer.WriteField("filename", filepath.Base(filePath))
		_ = writer.WriteField("worldName", meta.WorldName)
		_ = writer.WriteField("missionName", meta.MissionName)
		_ = writer.WriteField("missionDuration", fmt.Sprintf("%f", meta.MissionDuration))
		_ = writer.WriteField("tag", meta.Tag)
		if len(meta.FocusRanges) > 0 {
			first := meta.FocusRanges[0]
			_ = writer.WriteField("focusStart", strconv.FormatUint(uint64(first.Start), 10))
			_ = writer.WriteField("focusEnd", strconv.FormatUint(uint64(first.End), 10))
		}

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

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/v1/operations/add", pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check goroutine error
	if writeErr := <-errCh; writeErr != nil {
		return writeErr
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload returned status %d", resp.StatusCode)
	}
	return nil
}
