package mission

import (
	"sync"

	"github.com/OCAP2/extension/v5/internal/model"
)

// Context holds the current mission and world state
type Context struct {
	mu      sync.RWMutex
	Mission *model.Mission
	World   *model.World
}

// NewContext creates a new Context with default values
func NewContext() *Context {
	return &Context{
		Mission: &model.Mission{MissionName: "No mission loaded"},
		World:   &model.World{WorldName: "No world loaded"},
	}
}

// GetMission returns the current mission
func (mc *Context) GetMission() *model.Mission {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.Mission
}

// GetWorld returns the current world
func (mc *Context) GetWorld() *model.World {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.World
}

// SetMission sets the current mission and world
func (mc *Context) SetMission(mission *model.Mission, world *model.World) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.Mission = mission
	mc.World = world
}
