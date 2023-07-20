package defs

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

////////////////////////
// DATABASE STRUCTURES //
////////////////////////

////////////////////////
// SYSTEM MODELS
////////////////////////

// key value store for OCAP Group info, later use in frontend?
type OcapInfo struct {
	gorm.Model
	GroupName        string `json:"groupName" gorm:"size:127"` // primary key
	GroupDescription string `json:"groupDescription" gorm:"size:255"`
	GroupWebsite     string `json:"groupURL" gorm:"size:255"`
	GroupLogo        string `json:"groupLogoURL" gorm:"size:255"`
}

////////////////////////
// AAR MODELS
////////////////////////

type AfterActionReview struct {
	gorm.Model
	MissionID    uint    `json:"missionID"`
	Mission      Mission `gorm:"foreignkey:MissionID"`
	Author       string  `json:"author" gorm:"size:64"`
	Rating       float32 `json:"rating"`
	CommentGood  string  `json:"commentGood" gorm:"size:2000"`
	CommentBad   string  `json:"commentBad" gorm:"size:2000"`
	CommentOther string  `json:"commentOther" gorm:"size:2000"`
}

////////////////////////
// RECORDING MODELS
////////////////////////

type World struct {
	gorm.Model
	Author            string         `json:"author" gorm:"size:64"`
	WorkshopID        string         `json:"workshopID" gorm:"size:64"`
	DisplayName       string         `json:"displayName" gorm:"size:127"`
	WorldName         string         `json:"worldName" gorm:"size:127"`
	WorldNameOriginal string         `json:"worldNameOriginal" gorm:"size:127"`
	WorldSize         float32        `json:"worldSize"`
	Latitude          float32        `json:"latitude" gorm:"-"`
	Longitude         float32        `json:"longitude" gorm:"-"`
	Location          GPSCoordinates `json:"location"`
	Missions          []Mission
}

type Mission struct {
	gorm.Model
	MissionName                  string    `json:"missionName" gorm:"size:200"`
	BriefingName                 string    `json:"briefingName" gorm:"size:200"`
	MissionNameSource            string    `json:"missionNameSource" gorm:"size:200"`
	OnLoadName                   string    `json:"onLoadName" gorm:"size:200"`
	Author                       string    `json:"author" gorm:"size:200"`
	ServerName                   string    `json:"serverName" gorm:"size:200"`
	ServerProfile                string    `json:"serverProfile" gorm:"size:200"`
	StartTime                    time.Time `json:"missionStart" gorm:"type:timestamptz;index:idx_mission_start"` // time.Time
	WorldID                      uint
	World                        World     `gorm:"foreignkey:WorldID"`
	CaptureDelay                 float32   `json:"-" gorm:"default:1.0"`
	AddonVersion                 string    `json:"addonVersion" gorm:"size:64;default:2.0.0"`
	ExtensionVersion             string    `json:"extensionVersion" gorm:"size:64;default:2.0.0"`
	ExtensionBuild               string    `json:"extensionBuild" gorm:"size:64;default:2.0.0"`
	OcapRecorderExtensionVersion string    `json:"ocapRecorderExtensionVersion" gorm:"size:64;default:1.0.0"`
	Tag                          string    `json:"tag" gorm:"size:127"`
	Addons                       []Addon   `json:"-" gorm:"many2many:mission_addons;"`
	Soldiers                     []Soldier `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Vehicles                     []Vehicle `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerConnects    []EventPlayerConnect    `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	// EventPlayerDisconnects []EventPlayerDisconnect `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

type Addon struct {
	gorm.Model
	Missions   []Mission `gorm:"many2many:mission_addons;"`
	Name       string    `json:"name" gorm:"size:127"`
	WorkshopID string    `json:"workshopID" gorm:"size:127"`
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
	PlayerUID       string    `json:"playerUID" gorm:"size:64; default:NULL; index:idx_player_uid"`
	Type            string    `json:"type" gorm:"default:unit;size:16"`
	SoldierStates   []SoldierState
	FiredEvents     []FiredEvent
}

// SoldierState inherits from Frame
type SoldierState struct {
	// composite primary key with Time and OCAPID
	Time         time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	SoldierID    uint      `json:"soldierId" gorm:"primarykey;index:idx_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	MissionID    uint      `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"primarykey;"`

	Position         GPSCoordinates `json:"position"`
	ElevationASL     float32        `json:"elevationASL"`
	Bearing          uint16         `json:"bearing" gorm:"default:0"`
	Lifestate        uint8          `json:"lifestate" gorm:"default:0"`
	InVehicle        bool           `json:"inVehicle" gorm:"default:false"`
	UnitName         string         `json:"unitName" gorm:"size:64"`
	IsPlayer         bool           `json:"isPlayer" gorm:"default:false"`
	CurrentRole      string         `json:"currentRole" gorm:"size:64"`
	HasStableVitals  bool           `json:"hasStableVitals" gorm:"default:true"`
	IsDraggedCarried bool           `json:"isDraggedCarried" gorm:"default:false"`
	Scores           SoldierScores  `json:"scores" gorm:"embedded;embeddedPrefix:scores_"`
}

type SoldierScores struct {
	InfantryKills uint8 `json:"infantryKills"`
	VehicleKills  uint8 `json:"vehicleKills"`
	ArmorKills    uint8 `json:"armorKills"`
	AirKills      uint8 `json:"airKills"`
	Deaths        uint8 `json:"deaths"`
	TotalScore    uint8 `json:"totalScore"`
}

type Vehicle struct {
	gorm.Model
	Mission       Mission   `gorm:"foreignkey:MissionID"`
	MissionID     uint      `json:"missionId"`
	JoinTime      time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;"`
	JoinFrame     uint      `json:"joinFrame"`
	OcapID        uint16    `json:"ocapId" gorm:"index:idx_ocap_id"`
	OcapType      string    `json:"vehicleClass" gorm:"size:64"`
	ClassName     string    `json:"className" gorm:"size:64"`
	DisplayName   string    `json:"displayName" gorm:"size:64"`
	Customization string    `json:"customization"`
	VehicleStates []VehicleState
}

type VehicleState struct {
	// composite primary key with Time and OCAPID
	Time         time.Time      `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	VehicleID    uint           `json:"soldierID" gorm:"primarykey;index:idx_vehicle_id"`
	Vehicle      Vehicle        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`
	MissionID    uint           `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"primarykey;"`
	Position     GPSCoordinates `json:"position"`
	ElevationASL float32        `json:"elevationASL"`
	Bearing      uint16         `json:"bearing"`
	IsAlive      bool           `json:"isAlive"`
	Crew         string         `json:"crew" gorm:"size:128"`
}

// fired events
type FiredEvent struct {
	Time         time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	SoldierID    uint      `json:"soldierId" gorm:"primarykey;index:idx_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	MissionID    uint      `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_capture_frame;primarykey;"`
	Weapon       string    `json:"weapon" gorm:"size:64"`
	Magazine     string    `json:"magazine" gorm:"size:64"`
	FiringMode   string    `json:"mode" gorm:"size:64"`

	StartPosition     GPSCoordinates `json:"startPos"`
	StartElevationASL float32        `json:"startElev"`
	EndPosition       GPSCoordinates `json:"endPos"`
	EndElevationASL   float32        `json:"endElev"`
}

type GeneralEvent struct {
	Time         time.Time      `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	MissionID    uint           `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"index:idx_capture_frame;primarykey;"`
	Name         string         `json:"name" gorm:"size:64"`
	Message      string         `json:"message"`
	ExtraData    datatypes.JSON `json:"extraData" gorm:"type:jsonb;default:'{}'"`
}

type HitEvent struct {
	Time time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	// caused by could be soldier or vehicle
	VictimIDSoldier  uint    `json:"victimIdSoldier" gorm:"primarykey;index:idx_victim_id;default:NULL"`
	VictimSoldier    Soldier `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDSoldier;"`
	VictimIDVehicle  uint    `json:"victimIdVehicle" gorm:"primarykey;index:idx_victim_id;default:NULL"`
	VictimVehicle    Vehicle `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDVehicle;"`
	ShooterIDSoldier uint    `json:"shooterIdSoldier" gorm:"primarykey;index:idx_shooter_id"`
	ShooterSoldier   Soldier `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterIDSoldier;"`
	ShooterIDVehicle uint    `json:"shooterIdVehicle" gorm:"primarykey;index:idx_shooter_id"`
	ShooterVehicle   Vehicle `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterIDVehicle;"`

	MissionID    uint    `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_capture_frame;primarykey;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
}

type KillEvent struct {
	Time time.Time `json:"time" gorm:"type:timestamptz;NOT NULL;primarykey;"`
	// caused by could be soldier or vehicle
	VictimIDSoldier uint    `json:"victimIdSoldier" gorm:"primarykey;index:idx_victim_id;default:NULL"`
	VictimSoldier   Soldier `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDSoldier;"`
	VictimIDVehicle uint    `json:"victimIdVehicle" gorm:"primarykey;index:idx_victim_id;default:NULL"`
	VictimVehicle   Vehicle `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDVehicle;"`
	KillerIDSoldier uint    `json:"killerIdSoldier" gorm:"primarykey;index:idx_killer_id;default:NULL"`
	KillerSoldier   Soldier `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDSoldier;"`
	KillerIDVehicle uint    `json:"killerIdVehicle" gorm:"primarykey;index:idx_killer_id"`
	KillerVehicle   Vehicle `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDVehicle;"`

	MissionID    uint    `json:"missionId" gorm:"primarykey;index:idx_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_capture_frame;primarykey;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
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
