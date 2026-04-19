// internal/storage/memory/export.go
package memory

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/OCAP2/extension/v5/internal/storage/memory/export/v1"
)

// exportJSON writes the mission data to a gzipped JSON file.
// Caller must hold b.mu lock.
//
// Uses v1.Stream to write entities one at a time so peak memory during
// the save is bounded by the largest single entity's gap-filled
// positions, not the sum across all entities.
func (b *Backend) exportJSON() error {
	data := b.buildMissionDataUnlocked()

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
		if err := writeGzipJSON(outputPath, data); err != nil {
			return err
		}
	} else {
		if err := writeJSON(outputPath, data); err != nil {
			return err
		}
	}

	b.lastExportPath = outputPath
	return nil
}

func writeJSON(path string, data *v1.MissionData) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return v1.Stream(f, data)
}

func writeGzipJSON(path string, data *v1.MissionData) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	gzWriter := gzip.NewWriter(f)
	defer func() { _ = gzWriter.Close() }()

	return v1.Stream(gzWriter, data)
}
