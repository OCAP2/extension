// internal/storage/storage_test.go
package storage_test

import (
	"testing"

	"github.com/OCAP2/extension/v5/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestUploadMetadataFields(t *testing.T) {
	meta := core.UploadMetadata{
		WorldName:       "Altis",
		MissionName:     "Test Mission",
		MissionDuration: 3600.5,
		Tag:             "TvT",
	}

	assert.Equal(t, "Altis", meta.WorldName)
	assert.Equal(t, "Test Mission", meta.MissionName)
	assert.Equal(t, 3600.5, meta.MissionDuration)
	assert.Equal(t, "TvT", meta.Tag)
}
