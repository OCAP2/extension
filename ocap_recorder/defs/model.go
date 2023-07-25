package defs

import (
	"database/sql"
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
	GeneralEvents                []GeneralEvent
	HitEvents                    []HitEvent
	KillEvents                   []KillEvent
	FiredEvents                  []FiredEvent
	ChatEvents                   []ChatEvent
	RadioEvents                  []RadioEvent
	ServerFpsEvents              []ServerFpsEvent
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
	JoinTime        time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_join_time"`
	JoinFrame       uint      `json:"joinFrame"`
	OcapID          uint16    `json:"ocapId" gorm:"index:idx_ocap_id"`
	UnitName        string    `json:"unitName" gorm:"size:64"`
	GroupID         string    `json:"groupId" gorm:"size:64"`
	Side            string    `json:"side" gorm:"size:16"`
	IsPlayer        bool      `json:"isPlayer" gorm:"default:false"`
	RoleDescription string    `json:"roleDescription" gorm:"size:64"`
	PlayerUID       string    `json:"playerUID" gorm:"size:64; default:NULL; index:idx_player_uid"`
	ClassName       string    `json:"type" gorm:"default:NULL;size:64"`
	DisplayName     string    `json:"displayName" gorm:"default:NULL;size:64"`
	SoldierStates   []SoldierState
	FiredEvents     []FiredEvent
	ChatEvents      []ChatEvent
	RadioEvents     []RadioEvent
}

// SoldierState inherits from Frame
type SoldierState struct {
	// composite primary key with Time and OCAPID
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_capture_frame"`

	Position         GPSCoordinates `json:"position"`
	ElevationASL     float32        `json:"elevationASL"`
	Bearing          uint16         `json:"bearing" gorm:"default:0"`
	Lifestate        uint8          `json:"lifestate" gorm:"default:0"`
	InVehicle        bool           `json:"inVehicle" gorm:"default:false"`
	VehicleRole      string         `json:"vehicleRole" gorm:"size:64"`
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
	JoinTime      time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_join_time"`
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
	ID           uint           `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time      `json:"time" gorm:"type:timestamptz;"`
	VehicleID    uint           `json:"soldierID" gorm:"index:idx_vehicle_id"`
	Vehicle      Vehicle        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`
	MissionID    uint           `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"index:idx_capture_frame"`
	Position     GPSCoordinates `json:"position"`
	ElevationASL float32        `json:"elevationASL"`
	Bearing      uint16         `json:"bearing"`
	IsAlive      bool           `json:"isAlive"`
	Crew         string         `json:"crew" gorm:"size:128"`
	Fuel         float32        `json:"fuel"`
	Damage       float32        `json:"damage"`
	Locked       bool           `json:"locked"`
	EngineOn     bool           `json:"engineOn"`
	Side         string         `json:"side" gorm:"size:16"`
}

// fired events
type FiredEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_capture_frame;"`
	Weapon       string    `json:"weapon" gorm:"size:64"`
	Magazine     string    `json:"magazine" gorm:"size:64"`
	FiringMode   string    `json:"mode" gorm:"size:64"`

	StartPosition     GPSCoordinates `json:"startPos"`
	StartElevationASL float32        `json:"startElev"`
	EndPosition       GPSCoordinates `json:"endPos"`
	EndElevationASL   float32        `json:"endElev"`
}

type GeneralEvent struct {
	ID           uint           `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time      `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint           `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"index:idx_capture_frame;"`
	Name         string         `json:"name" gorm:"size:64"`
	Message      string         `json:"message"`
	ExtraData    datatypes.JSON `json:"extraData" gorm:"type:jsonb;default:'{}'"`
}

type HitEvent struct {
	ID   uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time time.Time `json:"time" gorm:"type:timestamptz;"`
	// caused by could be soldier or vehicle
	VictimIDSoldier  sql.NullInt32 `json:"victimIdSoldier" gorm:"index:idx_victim_id;default:NULL"`
	VictimSoldier    Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDSoldier;"`
	VictimIDVehicle  sql.NullInt32 `json:"victimIdVehicle" gorm:"index:idx_victim_id;default:NULL"`
	VictimVehicle    Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDVehicle;"`
	ShooterIDSoldier sql.NullInt32 `json:"shooterIdSoldier" gorm:"index:idx_shooter_id;default:NULL"`
	ShooterSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterIDSoldier;"`
	ShooterIDVehicle sql.NullInt32 `json:"shooterIdVehicle" gorm:"index:idx_shooter_id;default:NULL"`
	ShooterVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterIDVehicle;"`

	MissionID    uint    `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_capture_frame;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
}

type KillEvent struct {
	ID   uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time time.Time `json:"time" gorm:"type:timestamptz;"`
	// caused by could be soldier or vehicle
	VictimIDSoldier sql.NullInt32 `json:"victimIdSoldier" gorm:"index:idx_victim_id;default:NULL"`
	VictimSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDSoldier;"`
	VictimIDVehicle sql.NullInt32 `json:"victimIdVehicle" gorm:"index:idx_victim_id;default:NULL"`
	VictimVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDVehicle;"`
	KillerIDSoldier sql.NullInt32 `json:"killerIdSoldier" gorm:"index:idx_killer_id;default:NULL"`
	KillerSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDSoldier;"`
	KillerIDVehicle sql.NullInt32 `json:"killerIdVehicle" gorm:"index:idx_killer_id"`
	KillerVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDVehicle;"`

	MissionID    uint    `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_capture_frame;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
}

var ChatChannels map[int]string = map[int]string{
	0:  "Global",
	1:  "Side",
	2:  "Command",
	3:  "Group",
	4:  "Vehicle",
	5:  "Direct",
	16: "System",
}

type ChatEvent struct {
	ID           uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time     `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint          `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierID    sql.NullInt32 `json:"soldierId" gorm:"index:idx_soldier_id;default:NULL"`
	Soldier      Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint          `json:"captureFrame" gorm:"index:idx_capture_frame;"`
	Channel      string        `json:"channel" gorm:"size:64"`
	FromName     string        `json:"from" gorm:"size:64"`
	SenderName   string        `json:"name" gorm:"size:64"`
	Message      string        `json:"text"`
	PlayerUid    string        `json:"playerUID" gorm:"size:64; default:NULL; index:idx_player_uid"`
}

type RadioEvent struct {
	ID           uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time     `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint          `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierID    sql.NullInt32 `json:"soldierId" gorm:"index:idx_soldier_id;default:NULL"`
	Soldier      Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint          `json:"captureFrame" gorm:"index:idx_capture_frame;"`

	Radio        string  `json:"radio" gorm:"size:32"`
	RadioType    string  `json:"radioType" gorm:"size:8"`
	StartEnd     string  `json:"startEnd" gorm:"size:8"`
	Channel      int8    `json:"channel"`
	IsAdditional bool    `json:"isAdditional"`
	Frequency    float32 `json:"frequency"`
	Code         string  `json:"code" gorm:"size:32"`
}

type ServerFpsEvent struct {
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_capture_frame;"`
	FpsAverage   float32   `json:"fpsAvg"`
	FpsMin       float32   `json:"fpsMin"`
}
