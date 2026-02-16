// pkg/core/mission.go
package core

import "time"

// World represents a map/terrain
type World struct {
	ID                uint
	Author            string
	WorkshopID        string
	DisplayName       string
	WorldName         string
	WorldNameOriginal string
	WorldSize         float32
	Latitude          float32
	Longitude         float32
	Location          Position3D
}

// Mission represents a recorded mission
type Mission struct {
	ID                           uint
	MissionName                  string
	BriefingName                 string
	MissionNameSource            string
	OnLoadName                   string
	Author                       string
	ServerName                   string
	ServerProfile                string
	StartTime                    time.Time
	WorldID                      uint
	CaptureDelay                 float32
	AddonVersion                 string
	ExtensionVersion             string
	ExtensionBuild               string
	OcapRecorderExtensionVersion string
	Tag                          string
	PlayableSlots                PlayableSlots
	SideFriendly                 SideFriendly
	Addons                       []Addon
}

// Addon represents a mod or DLC
type Addon struct {
	ID         uint
	Name       string
	WorkshopID string
}
