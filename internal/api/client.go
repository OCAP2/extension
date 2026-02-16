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

	"github.com/OCAP2/extension/v5/pkg/core"
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
func (c *Client) Upload(filePath string, meta core.UploadMetadata) error {
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
