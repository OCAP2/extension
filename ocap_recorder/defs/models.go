package defs

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// /////////////////////
// DATABASE STRUCTURES //
// /////////////////////

// key value store for OCAP Group info, later use in frontend?
type OcapInfo struct {
	gorm.Model
	GroupName        string `json:"groupName" gorm:"size:127"` // primary key
	GroupDescription string `json:"groupDescription" gorm:"size:255"`
	GroupWebsite     string `json:"groupURL" gorm:"size:255"`
	GroupLogo        string `json:"groupLogoURL" gorm:"size:255"`
}

// setup status of the database
type DatabaseStatus struct {
	gorm.Model
	SetupTime             time.Time `json:"setupTime"`
	TablesMigrated        bool      `json:"tablesMigrated"`
	TablesMigratedTime    time.Time `json:"tablesMigratedTime"`
	HyperTablesConfigured bool      `json:"hyperTablesConfigured"`
}

type World struct {
	gorm.Model
	Author            string  `json:"author" gorm:"size:64"`
	WorkshopID        string  `json:"workshopID" gorm:"size:64"`
	DisplayName       string  `json:"displayName" gorm:"size:127"`
	WorldName         string  `json:"worldName" gorm:"size:127"`
	WorldNameOriginal string  `json:"worldNameOriginal" gorm:"size:127"`
	WorldSize         float32 `json:"worldSize"`
	Latitude          float32 `json:"latitude"`
	Longitude         float32 `json:"longitude"`
	Missions          []Mission
}

type Mission struct {
	gorm.Model
	MissionName       string    `json:"missionName" gorm:"size:200"`
	BriefingName      string    `json:"briefingName" gorm:"size:200"`
	MissionNameSource string    `json:"missionNameSource" gorm:"size:200"`
	OnLoadName        string    `json:"onLoadName" gorm:"size:200"`
	Author            string    `json:"author" gorm:"size:200"`
	ServerName        string    `json:"serverName" gorm:"size:200"`
	ServerProfile     string    `json:"serverProfile" gorm:"size:200"`
	StartTime         time.Time `json:"missionStart" gorm:"type:timestamptz;index:idx_mission_start"` // time.Time
	WorldName         string    `json:"worldName" gorm:"-"`
	WorldID           uint
	World             World     `gorm:"foreignkey:WorldID"`
	Tag               string    `json:"tag" gorm:"size:127"`
	Soldiers          []Soldier `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// Vehicles               []Vehicle               `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerConnects    []EventPlayerConnect    `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerDisconnects []EventPlayerDisconnect `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Soldier struct {
	gorm.Model
	Mission         Mission   `gorm:"foreignkey:MissionID"`
	MissionID       uint      `json:"missionId"`
	JoinTime        time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;"`
	JoinFrame       uint      `json:"joinFrame"`
	OcapID          uint16    `json:"ocapId" gorm:"index:idx_ocap_id"`
	UnitName        string    `json:"unitName" gorm:"size:64"`
	GroupID         string    `json:"groupId" gorm:"size:64"`
	Side            string    `json:"side" gorm:"size:16"`
	IsPlayer        bool      `json:"isPlayer" gorm:"default:false"`
	RoleDescription string    `json:"roleDescription" gorm:"size:64"`
}

// SoldierState inherits from Frame
type SoldierState struct {
	// composite primary key with Time and OCAPID
	Time         time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	SoldierID    uint      `json:"soldierId" gorm:"primarykey;index:idx_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint      `json:"captureFrame"`

	Position     GPSCoordinates `json:"position"`
	ElevationASL float32        `json:"elevationASL"`
	Bearing      uint16         `json:"bearing" gorm:"default:0"`
	Lifestate    uint8          `json:"lifestate" gorm:"default:0"`
	InVehicle    bool           `json:"inVehicle" gorm:"default:false"`
	UnitName     string         `json:"unitName" gorm:"size:64"`
	IsPlayer     bool           `json:"isPlayer" gorm:"default:false"`
	CurrentRole  string         `json:"currentRole" gorm:"size:64"`
}

type Vehicle struct {
	gorm.Model
	Mission      Mission   `gorm:"foreignkey:MissionID"`
	MissionID    uint      `json:"missionId"`
	JoinTime     time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;"`
	JoinFrame    uint      `json:"joinFrame"`
	OcapID       uint16    `json:"ocapId" gorm:"index:idx_ocap_id"`
	VehicleClass string    `json:"vehicleClass" gorm:"size:64"`
	DisplayName  string    `json:"displayName" gorm:"size:64"`
}

type VehicleState struct {
	// composite primary key with Time and OCAPID
	Time         time.Time      `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	VehicleID    uint           `json:"soldierID" gorm:"primarykey;index:idx_vehicle_id"`
	Vehicle      Vehicle        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`
	CaptureFrame uint           `json:"captureFrame"`
	Position     GPSCoordinates `json:"position"`
	ElevationASL float32        `json:"elevationASL"`
	Bearing      uint16         `json:"bearing"`
	IsAlive      bool           `json:"isAlive"`
	Crew         datatypes.JSON `json:"crew"`
}

// event types
type EventPlayerConnect struct {
	gorm.Model
	Mission      Mission `gorm:"foreignkey:MissionID"`
	MissionID    uint
	CaptureFrame uint32 `json:"captureFrame"`
	EventType    string `json:"eventType" gorm:"size:32"`
	PlayerUID    string `json:"playerUid" gorm:"index:idx_player_uid;size:20"`
	ProfileName  string `json:"playerName" gorm:"size:32"`
}

type EventPlayerDisconnect struct {
	gorm.Model
	Mission      Mission `gorm:"foreignkey:MissionID"`
	MissionID    uint
	CaptureFrame uint32 `json:"captureFrame"`
	EventType    string `json:"eventType" gorm:"size:32"`
	PlayerUID    string `json:"playerUid" gorm:"index:idx_player_uid;size:20"`
	ProfileName  string `json:"playerName" gorm:"size:32"`
}
