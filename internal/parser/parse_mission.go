package parser

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/OCAP2/extension/v5/internal/geo"
	"github.com/OCAP2/extension/v5/internal/model"
	"github.com/OCAP2/extension/v5/internal/util"
)

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
	addonsRaw, ok := missionTemp["addons"].([]any)
	if !ok {
		return mission, world, fmt.Errorf("addons field is missing or not an array")
	}
	addons := []model.Addon{}
	for _, addon := range addonsRaw {
		addonInfo, ok := addon.([]any)
		if !ok || len(addonInfo) < 2 {
			return mission, world, fmt.Errorf("invalid addon format")
		}
		addonName, ok := addonInfo[0].(string)
		if !ok {
			return mission, world, fmt.Errorf("invalid addon name type")
		}
		thisAddon := model.Addon{Name: addonName}
		switch v := addonInfo[1].(type) {
		case float64:
			intVal := int64(v)
			if v != float64(intVal) {
				return mission, world, fmt.Errorf("invalid addon workshop ID: float has fractional part %f", v)
			}
			thisAddon.WorkshopID = strconv.FormatInt(intVal, 10)
		case string:
			thisAddon.WorkshopID = v
		default:
			return mission, world, fmt.Errorf("invalid addon workshop ID type: %T", addonInfo[1])
		}
		addons = append(addons, thisAddon)
	}
	mission.Addons = addons

	mission.StartTime = time.Now()

	// Helper to safely extract typed fields from the mission map
	getString := func(key string) (string, error) {
		v, ok := missionTemp[key].(string)
		if !ok {
			return "", fmt.Errorf("mission field %q is missing or not a string", key)
		}
		return v, nil
	}
	getFloat := func(key string) (float64, error) {
		v, ok := missionTemp[key].(float64)
		if !ok {
			return 0, fmt.Errorf("mission field %q is missing or not a number", key)
		}
		return v, nil
	}
	getSlice := func(key string) ([]any, error) {
		v, ok := missionTemp[key].([]any)
		if !ok {
			return nil, fmt.Errorf("mission field %q is missing or not an array", key)
		}
		return v, nil
	}

	captureDelay, err := getFloat("captureDelay")
	if err != nil {
		return mission, world, err
	}
	mission.CaptureDelay = float32(captureDelay)

	if mission.MissionNameSource, err = getString("missionNameSource"); err != nil {
		return mission, world, err
	}
	if mission.MissionName, err = getString("missionName"); err != nil {
		return mission, world, err
	}
	if mission.BriefingName, err = getString("briefingName"); err != nil {
		return mission, world, err
	}
	if mission.ServerName, err = getString("serverName"); err != nil {
		return mission, world, err
	}
	if mission.ServerProfile, err = getString("serverProfile"); err != nil {
		return mission, world, err
	}
	if mission.OnLoadName, err = getString("onLoadName"); err != nil {
		return mission, world, err
	}
	if mission.Author, err = getString("author"); err != nil {
		return mission, world, err
	}
	if mission.Tag, err = getString("tag"); err != nil {
		return mission, world, err
	}

	// playableSlots
	playableSlotsJSON, err := getSlice("playableSlots")
	if err != nil {
		return mission, world, err
	}
	if len(playableSlotsJSON) < 5 {
		return mission, world, fmt.Errorf("playableSlots needs 5 elements, got %d", len(playableSlotsJSON))
	}
	mission.PlayableSlots.East = uint8(playableSlotsJSON[0].(float64))
	mission.PlayableSlots.West = uint8(playableSlotsJSON[1].(float64))
	mission.PlayableSlots.Independent = uint8(playableSlotsJSON[2].(float64))
	mission.PlayableSlots.Civilian = uint8(playableSlotsJSON[3].(float64))
	mission.PlayableSlots.Logic = uint8(playableSlotsJSON[4].(float64))

	// sideFriendly
	sideFriendlyJSON, err := getSlice("sideFriendly")
	if err != nil {
		return mission, world, err
	}
	if len(sideFriendlyJSON) < 3 {
		return mission, world, fmt.Errorf("sideFriendly needs 3 elements, got %d", len(sideFriendlyJSON))
	}
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
