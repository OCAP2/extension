package parser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"
)

// parseUintFromFloat parses a string that may be an integer ("32") or float ("32.00") into uint64.
// ArmA 3's SQF has no integer type, so the extension API may serialize numbers as floats.
func parseUintFromFloat(s string) (uint64, error) {
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f < 0 || f != float64(uint64(f)) {
		return 0, fmt.Errorf("parseUintFromFloat: %q is not a valid uint64", s)
	}
	return uint64(f), nil
}

// parseIntFromFloat parses a string that may be an integer or float into int64.
func parseIntFromFloat(s string) (int64, error) {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if f != float64(int64(f)) {
		return 0, fmt.Errorf("parseIntFromFloat: %q is not a valid int64", s)
	}
	return int64(f), nil
}

// MissionContext holds the current mission and world state
type MissionContext struct {
	mu      sync.RWMutex
	Mission *model.Mission
	World   *model.World
}

// NewMissionContext creates a new MissionContext with default values
func NewMissionContext() *MissionContext {
	return &MissionContext{
		Mission: &model.Mission{MissionName: "No mission loaded"},
		World:   &model.World{WorldName: "No world loaded"},
	}
}

// GetMission returns the current mission
func (mc *MissionContext) GetMission() *model.Mission {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.Mission
}

// GetWorld returns the current world
func (mc *MissionContext) GetWorld() *model.World {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.World
}

// SetMission sets the current mission and world
func (mc *MissionContext) SetMission(mission *model.Mission, world *model.World) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.Mission = mission
	mc.World = world
}

// Parser provides pure []string -> model struct conversion.
// It has zero external dependencies beyond a logger.
type Parser struct {
	logger  *slog.Logger
	mission atomic.Pointer[model.Mission]

	// Static config set at creation time
	addonVersion     string
	extensionVersion string
}

// NewParser creates a new parser with only a logger dependency
func NewParser(logger *slog.Logger, addonVersion, extensionVersion string) *Parser {
	p := &Parser{
		logger:           logger,
		addonVersion:     addonVersion,
		extensionVersion: extensionVersion,
	}
	return p
}

// SetMission sets the current mission for MissionID lookups
func (p *Parser) SetMission(m *model.Mission) {
	p.mission.Store(m)
}

func (p *Parser) getMissionID() uint {
	m := p.mission.Load()
	if m == nil {
		return 0
	}
	return m.ID
}

// ParseMission parses mission and world data from raw args.
// Returns parsed mission + world. NO DB operations, NO cache resets, NO callbacks.
func (p *Parser) ParseMission(data []string) (model.Mission, model.World, error) {
	var mission model.Mission
	var world model.World

	// fix received data
	for i, v := range data {
		data[i] = util.FixEscapeQuotes(util.TrimQuotes(v))
	}

	// unmarshal data[0] -> world
	err := json.Unmarshal([]byte(data[0]), &world)
	if err != nil {
		return mission, world, fmt.Errorf("error unmarshalling world data: %w", err)
	}

	// preprocess the world 'location' to geopoint
	worldLocation, err := geo.Coords3857From4326(
		float64(world.Longitude),
		float64(world.Latitude),
	)
	if err != nil {
		return mission, world, fmt.Errorf("error converting world location to geopoint: %w", err)
	}
	world.Location = worldLocation

	// unmarshal data[1] -> mission (via temp map for addons extraction)
	missionTemp := map[string]any{}
	if err = json.Unmarshal([]byte(data[1]), &missionTemp); err != nil {
		return mission, world, fmt.Errorf("error unmarshalling mission data: %w", err)
	}

	// extract addons (without DB lookup - just parse)
	addons := []model.Addon{}
	for _, addon := range missionTemp["addons"].([]any) {
		thisAddon := model.Addon{
			Name: addon.([]any)[0].(string),
		}
		if reflect.TypeOf(addon.([]any)[1]).Kind() == reflect.Float64 {
			thisAddon.WorkshopID = strconv.Itoa(int(addon.([]any)[1].(float64)))
		} else {
			thisAddon.WorkshopID = addon.([]any)[1].(string)
		}
		addons = append(addons, thisAddon)
	}
	mission.Addons = addons

	mission.StartTime = time.Now()

	mission.CaptureDelay = float32(missionTemp["captureDelay"].(float64))
	mission.MissionNameSource = missionTemp["missionNameSource"].(string)
	mission.MissionName = missionTemp["missionName"].(string)
	mission.BriefingName = missionTemp["briefingName"].(string)
	mission.ServerName = missionTemp["serverName"].(string)
	mission.ServerProfile = missionTemp["serverProfile"].(string)
	mission.OnLoadName = missionTemp["onLoadName"].(string)
	mission.Author = missionTemp["author"].(string)
	mission.Tag = missionTemp["tag"].(string)

	// playableSlots
	playableSlotsJSON := missionTemp["playableSlots"].([]any)
	mission.PlayableSlots.East = uint8(playableSlotsJSON[0].(float64))
	mission.PlayableSlots.West = uint8(playableSlotsJSON[1].(float64))
	mission.PlayableSlots.Independent = uint8(playableSlotsJSON[2].(float64))
	mission.PlayableSlots.Civilian = uint8(playableSlotsJSON[3].(float64))
	mission.PlayableSlots.Logic = uint8(playableSlotsJSON[4].(float64))

	// sideFriendly
	sideFriendlyJSON := missionTemp["sideFriendly"].([]any)
	mission.SideFriendly.EastWest = sideFriendlyJSON[0].(bool)
	mission.SideFriendly.EastIndependent = sideFriendlyJSON[1].(bool)
	mission.SideFriendly.WestIndependent = sideFriendlyJSON[2].(bool)

	// received at extension init and saved to local memory
	mission.AddonVersion = p.addonVersion
	mission.ExtensionVersion = p.extensionVersion

	p.logger.Debug("Parsed mission data",
		"missionName", mission.MissionName,
		"worldName", world.WorldName)

	return mission, world, nil
}
