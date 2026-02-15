package mission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext_ThreadSafe(t *testing.T) {
	ctx := NewContext()

	m := ctx.GetMission()
	assert.Equal(t, "No mission loaded", m.MissionName)

	world := ctx.GetWorld()
	assert.Equal(t, "No world loaded", world.WorldName)
}
