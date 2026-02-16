// internal/storage/memory/export.go
package memory

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/OCAP2/extension/v5/internal/storage/memory/export/v1"
)

// exportJSON writes the mission data to a gzipped JSON file
// Caller must hold b.mu lock.
func (b *Backend) exportJSON() error {
	export := b.buildExportUnlocked()

	// Build filename
	missionName := strings.ReplaceAll(b.mission.MissionName, " ", "_")
	missionName = strings.ReplaceAll(missionName, ":", "_")
	timestamp := time.Now().Format("20060102_150405")

	var filename string
	if b.cfg.CompressOutput {
		filename = fmt.Sprintf("%s_%s.json.gz", missionName, timestamp)
	} else {
		filename = fmt.Sprintf("%s_%s.json", missionName, timestamp)
	}

	outputPath := filepath.Join(b.cfg.OutputDir, filename)

	// Ensure output directory exists
	if err := os.MkdirAll(b.cfg.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if b.cfg.CompressOutput {
		if err := writeGzipJSON(outputPath, export); err != nil {
			return err
		}
	} else {
		if err := writeJSON(outputPath, export); err != nil {
			return err
		}
	}

	b.lastExportPath = outputPath
	return nil
}

func writeJSON(path string, data v1.Export) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	encoder := json.NewEncoder(f)
	return encoder.Encode(data)
}

func writeGzipJSON(path string, data v1.Export) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	gzWriter := gzip.NewWriter(f)
	defer func() {
		if cerr := gzWriter.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	encoder := json.NewEncoder(gzWriter)
	return encoder.Encode(data)
}
