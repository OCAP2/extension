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
