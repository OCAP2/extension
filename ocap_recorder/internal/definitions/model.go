package ocapdefs

import (
	"database/sql"
	"time"

	geom "github.com/peterstace/simplefeatures/geom"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

////////////////////////
// DATABASE STRUCTURES //
////////////////////////

// DatabaseModels is a list of all the structs exported here which represent tables in the database schema
var DatabaseModels = []interface{}{
	&OcapInfo{},
	&AfterActionReview{},
	&World{},
	&Mission{},
	&Soldier{},
	&SoldierState{},
	&Vehicle{},
	&VehicleState{},
	&FiredEvent{},
	&ProjectileEvent{},
	&ProjectileHitsSoldier{},
	&ProjectileHitsVehicle{},
	&GeneralEvent{},
	&HitEvent{},
	&KillEvent{},
	&ChatEvent{},
	&RadioEvent{},
	&ServerFpsEvent{},
	&Ace3DeathEvent{},
	&Ace3UnconsciousEvent{},
	&OcapPerformance{},
	&Marker{},
	&MarkerState{},
}

var DatabaseModelsSQLite = []interface{}{
	&OcapInfo{},
	&AfterActionReview{},
	&World{},
	&Mission{},
	&Soldier{},
	&SoldierState{},
	&Vehicle{},
	&VehicleState{},
	&FiredEvent{},
	&GeneralEvent{},
	&HitEvent{},
	&KillEvent{},
	&ChatEvent{},
	&RadioEvent{},
	&ServerFpsEvent{},
	&Ace3DeathEvent{},
	&Ace3UnconsciousEvent{},
	&OcapPerformance{},
	&Marker{},
	&MarkerState{},
}

////////////////////////
// SYSTEM MODELS
////////////////////////

// OcapInfo contains group information about the instance
type OcapInfo struct {
	gorm.Model
	GroupName        string `json:"groupName" gorm:"size:127"` // primary key
	GroupDescription string `json:"groupDescription" gorm:"size:255"`
	GroupWebsite     string `json:"groupURL" gorm:"size:255"`
	GroupLogo        string `json:"groupLogoURL" gorm:"size:255"`
}

func (*OcapInfo) TableName() string {
	return "ocap_infos"
}

// OcapPerformance is the model for extension performance metrics
type OcapPerformance struct {
	Time                time.Time         `json:"time" gorm:"type:timestamptz;index:idx_time"`
	MissionID           uint              `json:"missionId" gorm:"index:idx_ocapperformance_mission_id"`
	Mission             Mission           `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	BufferLengths       BufferLengths     `json:"bufferLengths" gorm:"embedded;embeddedPrefix:buffer_"`
	WriteQueueLengths   WriteQueueLengths `json:"writeQueueLengths" gorm:"embedded;embeddedPrefix:writequeue_"`
	LastWriteDurationMs float32           `json:"lastWriteDurationMs"`
}

func (*OcapPerformance) TableName() string {
	return "ocap_performances"
}

// BufferLengths is the model for the buffer lengths
type BufferLengths struct {
	Soldiers              uint16 `json:"soldiers"`
	Vehicles              uint16 `json:"vehicles"`
	SoldierStates         uint16 `json:"soldierStates"`
	VehicleStates         uint16 `json:"vehicleStates"`
	GeneralEvents         uint16 `json:"generalEvents"`
	FiredEvents           uint16 `json:"firedEvents"`
	FiredEventsNew        uint16 `json:"firedEventsNew"`
	HitEvents             uint16 `json:"hitEvents"`
	KillEvents            uint16 `json:"killEvents"`
	ChatEvents            uint16 `json:"chatEvents"`
	RadioEvents           uint16 `json:"radioEvents"`
	ServerFpsEvents       uint16 `json:"serverFpsEvents"`
	Ace3DeathEvents       uint16 `json:"ace3DeathEvents"`
	Ace3UnconsciousEvents uint16 `json:"ace3UnconsciousEvents"`
}

// WriteQueueLengths is the model for the write queue lengths
type WriteQueueLengths struct {
	Soldiers              uint16 `json:"soldiers"`
	Vehicles              uint16 `json:"vehicles"`
	SoldierStates         uint16 `json:"soldierStates"`
	VehicleStates         uint16 `json:"vehicleStates"`
	GeneralEvents         uint16 `json:"generalEvents"`
	FiredEvents           uint16 `json:"firedEvents"`
	FiredEventsNew        uint16 `json:"firedEventsNew"`
	HitEvents             uint16 `json:"hitEvents"`
	KillEvents            uint16 `json:"killEvents"`
	ChatEvents            uint16 `json:"chatEvents"`
	RadioEvents           uint16 `json:"radioEvents"`
	ServerFpsEvents       uint16 `json:"serverFpsEvents"`
	Ace3DeathEvents       uint16 `json:"ace3DeathEvents"`
	Ace3UnconsciousEvents uint16 `json:"ace3UnconsciousEvents"`
}

func (b *BufferLengths) TableName() string {
	return "buffer_lengths"
}

////////////////////////
// RETRIEVAL
////////////////////////

// FrameData is the main model for a frame
type FrameData struct {
	OcapID       uint16         `json:"ocapId"`
	CaptureFrame uint           `json:"captureFrame"`
	States       datatypes.JSON `json:"states"`
	Hits         datatypes.JSON `json:"hits"`
	Kills        datatypes.JSON `json:"kills"`
	Fired        datatypes.JSON `json:"fired"`
	Radio        datatypes.JSON `json:"radio"`
	Chat         datatypes.JSON `json:"chat"`
}

func (*FrameData) TableName() string {
	return "frame_data"
}

////////////////////////
// AAR MODELS
////////////////////////

// AfterActionReview is the main model for an AAR filed by players
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

func (*AfterActionReview) TableName() string {
	return "after_action_reviews"
}

////////////////////////
// RECORDING MODELS
////////////////////////

// World is the main model for a world
type World struct {
	gorm.Model
	Author            string     `json:"author" gorm:"size:64"`
	WorkshopID        string     `json:"workshopID" gorm:"size:64"`
	DisplayName       string     `json:"displayName" gorm:"size:127"`
	WorldName         string     `json:"worldName" gorm:"size:127"`
	WorldNameOriginal string     `json:"worldNameOriginal" gorm:"size:127"`
	WorldSize         float32    `json:"worldSize"`
	Latitude          float32    `json:"latitude" gorm:"-"`
	Longitude         float32    `json:"longitude" gorm:"-"`
	Location          geom.Point `json:"location"`
	Missions          []Mission
}

func (*World) TableName() string {
	return "worlds"
}

func (w *World) GetOrInsert(db *gorm.DB) (
	created bool,
	err error,
) {
	var existingWorld World
	err = db.Where("world_name = ?", w.WorldName).First(&existingWorld).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// insert
			err = db.Create(w).Error
			return true, err
		}
		return false, err
	}
	// overwrite with db record if found
	*w = existingWorld
	return false, nil
}

// Mission is the main model for a mission
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
	World                        World   `gorm:"foreignkey:WorldID"`
	CaptureDelay                 float32 `json:"-" gorm:"default:1.0"`
	AddonVersion                 string  `json:"addonVersion" gorm:"size:64;default:2.0.0"`
	ExtensionVersion             string  `json:"extensionVersion" gorm:"size:64;default:2.0.0"`
	ExtensionBuild               string  `json:"extensionBuild" gorm:"size:64;default:2.0.0"`
	OcapRecorderExtensionVersion string  `json:"ocapRecorderExtensionVersion" gorm:"size:64;default:1.0.0"`
	Tag                          string  `json:"tag" gorm:"size:127"`

	Addons                []Addon       `json:"-" gorm:"many2many:mission_addons;"`
	Soldiers              []Soldier     `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Vehicles              []Vehicle     `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	PlayableSlots         PlayableSlots `json:"playableSlots" gorm:"embedded;embeddedPrefix:playable_"`
	SideFriendly          SideFriendly  `json:"sideFriendly" gorm:"embedded;embeddedPrefix:sidefriendly_"`
	GeneralEvents         []GeneralEvent
	HitEvents             []HitEvent
	KillEvents            []KillEvent
	FiredEvents           []FiredEvent
	ProjectileEvents      []ProjectileEvent
	ChatEvents            []ChatEvent
	RadioEvents           []RadioEvent
	ServerFpsEvents       []ServerFpsEvent
	Ace3DeathEvents       []Ace3DeathEvent
	Ace3UnconsciousEvents []Ace3UnconsciousEvent
	Markers               []Marker
	MarkerStates          []MarkerState
}

func (*Mission) TableName() string {
	return "missions"
}

// PlayableSlots shows counts of playable slots in the mission by side
type PlayableSlots struct {
	West        uint8 `json:"west"`
	East        uint8 `json:"east"`
	Independent uint8 `json:"independent"`
	Civilian    uint8 `json:"civilian"`
	Logic       uint8 `json:"logic"`
}

// SideFriendly represents which sides are allied
type SideFriendly struct {
	EastWest        bool `json:"eastWest"`
	EastIndependent bool `json:"eastIndependent"`
	WestIndependent bool `json:"westIndependent"`
}

// Addon is a mod or DLC
type Addon struct {
	gorm.Model
	Missions   []Mission `gorm:"many2many:mission_addons;"`
	Name       string    `json:"name" gorm:"size:127"`
	WorkshopID string    `json:"workshopID" gorm:"size:127"`
}

func (*Addon) TableName() string {
	return "addons"
}

// Soldier is a player or AI unit
type Soldier struct {
	gorm.Model
	Mission                     Mission        `gorm:"foreignkey:MissionID"`
	MissionID                   uint           `json:"missionId"`
	JoinTime                    time.Time      `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_soldier_join_time"`
	JoinFrame                   uint           `json:"joinFrame"`
	OcapID                      uint16         `json:"ocapId" gorm:"index:idx_soldier_ocap_id"`
	OcapType                    string         `json:"type" gorm:"size:16;default:man"`
	UnitName                    string         `json:"unitName" gorm:"size:64"`
	GroupID                     string         `json:"groupId" gorm:"size:64"`
	Side                        string         `json:"side" gorm:"size:16"`
	IsPlayer                    bool           `json:"isPlayer" gorm:"default:false"`
	RoleDescription             string         `json:"roleDescription" gorm:"size:64"`
	SquadParams                 datatypes.JSON `json:"squadParams" gorm:"type:jsonb;default:'[]'"`
	PlayerUID                   string         `json:"playerUID" gorm:"size:64; default:NULL; index:idx_soldier_player_uid"`
	ClassName                   string         `json:"className" gorm:"default:NULL;size:64"`
	DisplayName                 string         `json:"displayName" gorm:"default:NULL;size:64"`
	SoldierStates               []SoldierState
	FiredEvents                 []FiredEvent
	ProjectileEventsFirer       []ProjectileEvent `gorm:"foreignkey:FirerID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ProjectileEventsActualFirer []ProjectileEvent `gorm:"foreignkey:ActualFirerID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ChatEvents                  []ChatEvent
	RadioEvents                 []RadioEvent
}

func (*Soldier) TableName() string {
	return "soldiers"
}

func (s *Soldier) Get(db *gorm.DB) (err error) {
	err = db.Where(&s).Order(
		"join_time DESC",
	).First(&s).Error
	return err
}

// SoldierState inherits from Frame
type SoldierState struct {
	// composite primary key with Time and OCAPID
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_soldierstate_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_capture_frame"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_soldierstate_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`

	Position          geom.Point    `json:"position"`
	ElevationASL      float32       `json:"elevationASL"`
	Bearing           uint16        `json:"bearing" gorm:"default:0"`
	Lifestate         uint8         `json:"lifestate" gorm:"default:0"`
	InVehicle         bool          `json:"inVehicle" gorm:"default:false"`
	InVehicleObjectID sql.NullInt32 `json:"inVehicleObjectId" gorm:"default:NULL"`
	InVehicleObject   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:InVehicleObjectID;"`
	VehicleRole       string        `json:"vehicleRole" gorm:"size:64"`
	UnitName          string        `json:"unitName" gorm:"size:64"`
	IsPlayer          bool          `json:"isPlayer" gorm:"default:false"`
	CurrentRole       string        `json:"currentRole" gorm:"size:64"`
	HasStableVitals   bool          `json:"hasStableVitals" gorm:"default:true"`
	IsDraggedCarried  bool          `json:"isDraggedCarried" gorm:"default:false"`
	Stance            string        `json:"stance" gorm:"size:64"`
	Scores            SoldierScores `json:"scores" gorm:"embedded;embeddedPrefix:scores_"`
}

func (*SoldierState) TableName() string {
	return "soldier_states"
}

// SoldierScores stores Arma 3 player scores
type SoldierScores struct {
	InfantryKills uint8 `json:"infantryKills"`
	VehicleKills  uint8 `json:"vehicleKills"`
	ArmorKills    uint8 `json:"armorKills"`
	AirKills      uint8 `json:"airKills"`
	Deaths        uint8 `json:"deaths"`
	TotalScore    uint8 `json:"totalScore"`
}

// Vehicle is a vehicle or static weapon
type Vehicle struct {
	gorm.Model
	Mission       Mission   `gorm:"foreignkey:MissionID"`
	MissionID     uint      `json:"missionId"`
	JoinTime      time.Time `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_vehicle_join_time"`
	JoinFrame     uint      `json:"joinFrame"`
	OcapID        uint16    `json:"ocapId" gorm:"index:idx_vehicle_ocap_id"`
	OcapType      string    `json:"vehicleClass" gorm:"size:64"`
	ClassName     string    `json:"className" gorm:"size:64"`
	DisplayName   string    `json:"displayName" gorm:"size:64"`
	Customization string    `json:"customization"`
	VehicleStates []VehicleState
}

func (*Vehicle) TableName() string {
	return "vehicles"
}

// VehicleState defines the state of a vehicle at a given time
type VehicleState struct {
	// composite primary key with Time and OCAPID
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_vehiclestate_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_vehiclestate_capture_frame"`
	VehicleID    uint      `json:"soldierID" gorm:"index:idx_vehicle_id"`
	Vehicle      Vehicle   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`

	Position        geom.Point `json:"position"`
	ElevationASL    float32    `json:"elevationASL"`
	Bearing         uint16     `json:"bearing"`
	IsAlive         bool       `json:"isAlive"`
	Crew            string     `json:"crew" gorm:"size:128"`
	Fuel            float32    `json:"fuel"`
	Damage          float32    `json:"damage"`
	Locked          bool       `json:"locked"`
	EngineOn        bool       `json:"engineOn"`
	Side            string     `json:"side" gorm:"size:16"`
	VectorDir       string     `json:"vectorDir" gorm:"size:64"`
	VectorUp        string     `json:"vectorUp" gorm:"size:64"`
	TurretAzimuth   float32    `json:"turretAzimuth"`
	TurretElevation float32    `json:"turretElevation"`
}

func (*VehicleState) TableName() string {
	return "vehicle_states"
}

// FiredEvent represents a weapon being fired
type FiredEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_firedevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_firedevent_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_firedevent_capture_frame;"`
	Weapon       string    `json:"weapon" gorm:"size:64"`
	Magazine     string    `json:"magazine" gorm:"size:64"`
	FiringMode   string    `json:"mode" gorm:"size:64"`

	StartPosition     geom.Point `json:"startPos"`
	StartElevationASL float32    `json:"startElev"`
	EndPosition       geom.Point `json:"endPos"`
	EndElevationASL   float32    `json:"endElev"`
}

func (*FiredEvent) TableName() string {
	return "fired_events"
}

// FiredEventNew represents a weapon being fired and its lifetime
type ProjectileEvent struct {
	ID            uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time          time.Time `json:"firedTime" gorm:"type:timestamptz;"`
	MissionID     uint      `json:"missionId" gorm:"index:idx_projectile_mission_id"`
	Mission       Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	FirerID       uint      `json:"firerID" gorm:"index:idx_projectile_firer_id"`
	Firer         Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:FirerID;"`
	ActualFirerID uint      `json:"remoteControllerID" gorm:"index:idx_projectile_actual_firer_id"`
	ActualFirer   Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ActualFirerID;"`
	VehicleRole   string    `json:"vehicleRole" gorm:"size:32"`
	// omit the vehicle if nil, implying the soldier was not in one
	VehicleID    sql.NullInt32 `json:"vehicleID,omitempty" gorm:"index:idx_projectile_vehicle_id"`
	Vehicle      Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`
	CaptureFrame uint          `json:"firedFrame" gorm:"index:idx_projectile_capture_frame;"`

	Positions geom.Geometry `json:"-"`

	// projectile data
	InitialVelocity string `json:"initialVelocity" gorm:"size:64"`
	Weapon          string `json:"weapon" gorm:"size:64"`
	WeaponDisplay   string `json:"weaponDisplay" gorm:"size:64"`
	Magazine        string `json:"magazine" gorm:"size:64"`
	MagazineDisplay string `json:"magazineDisplay" gorm:"size:64"`
	Muzzle          string `json:"muzzle" gorm:"size:64"`
	MuzzleDisplay   string `json:"muzzleDisplay" gorm:"size:64"`
	Ammo            string `json:"ammo" gorm:"size:64"`
	Mode            string `json:"mode" gorm:"size:32"`

	// projectile hits
	HitSoldiers []ProjectileHitsSoldier `json:"hitsSoldiers" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	HitVehicles []ProjectileHitsVehicle `json:"hitsVehicles" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (p *ProjectileEvent) TableName() string {
	return "projectile_events"
}

type ProjectileHitsSoldier struct {
	ID                uint            `json:"id" gorm:"primarykey;autoIncrement;"`
	ProjectileEventID uint            `json:"projectileEventId" gorm:"index:idx_projectile_hit_soldier_projectile_event_id"`
	ProjectileEvent   ProjectileEvent `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ProjectileEventID;"`
	SoldierID         uint            `json:"soldierId" gorm:"index:idx_projectile_hit_soldier_soldier_id"`
	Soldier           Soldier         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame      uint            `json:"captureFrame" gorm:"index:idx_projectile_hit_soldier_capture_frame;"`
	Position          geom.Point      `json:"position"`
	ComponentsHit     datatypes.JSON  `json:"componentsHit"`
}

type ProjectileHitsVehicle struct {
	ID                uint            `json:"id" gorm:"primarykey;autoIncrement;"`
	ProjectileEventID uint            `json:"projectileEventId" gorm:"index:idx_projectile_hit_vehicle_projectile_event_id"`
	ProjectileEvent   ProjectileEvent `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ProjectileEventID;"`
	VehicleID         uint            `json:"vehicleId" gorm:"index:idx_projectile_hit_vehicle_vehicle_id"`
	Vehicle           Vehicle         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VehicleID;"`
	CaptureFrame      uint            `json:"captureFrame" gorm:"index:idx_projectile_hit_vehicle_capture_frame;"`
	Position          geom.Point      `json:"position"`
	ComponentsHit     datatypes.JSON  `json:"componentsHit"`
}

// GeneralEvent is a generic event that can be used to store any data
type GeneralEvent struct {
	ID           uint           `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time      `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint           `json:"missionId" gorm:"index:idx_generalevent_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"index:idx_generalevent_capture_frame;"`
	Name         string         `json:"name" gorm:"size:64"`
	Message      string         `json:"message"`
	ExtraData    datatypes.JSON `json:"extraData" gorm:"type:jsonb;default:'{}'"`
}

func (g *GeneralEvent) TableName() string {
	return "general_events"
}

// HitEvent represents something being hit by a projectile or explosion
type HitEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_hitevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_hitevent_capture_frame;"`

	// caused by could be soldier or vehicle
	VictimSoldierID  sql.NullInt32 `json:"victimSoldierId" gorm:"index:idx_hitevent_victim_id_soldier;default:NULL"`
	VictimSoldier    Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimSoldierID;"`
	VictimVehicleID  sql.NullInt32 `json:"victimVehicleId" gorm:"index:idx_hitevent_victim_id_vehicle;default:NULL"`
	VictimVehicle    Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimVehicleID;"`
	ShooterSoldierID sql.NullInt32 `json:"shooterSoldierId" gorm:"index:idx_shooter_id;default:NULL"`
	ShooterSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterSoldierID;"`
	ShooterVehicleID sql.NullInt32 `json:"shoooterVehicleId" gorm:"index:idx_shooter_id;default:NULL"`
	ShooterVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ShooterVehicleID;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
}

func (h *HitEvent) TableName() string {
	return "hit_events"
}

// KillEvent represents something being killed
type KillEvent struct {
	ID   uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time time.Time `json:"time" gorm:"type:timestamptz;"`

	MissionID    uint    `json:"missionId" gorm:"index:idx_killevent_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_killevent_capture_frame;"`

	// caused by could be soldier or vehicle
	VictimIDSoldier sql.NullInt32 `json:"victimIdSoldier" gorm:"index:idx_killevent_victim_id_soldier;default:NULL"`
	VictimSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDSoldier;"`
	VictimIDVehicle sql.NullInt32 `json:"victimIdVehicle" gorm:"index:idx_killevent_victim_id_vehicle;default:NULL"`
	VictimVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:VictimIDVehicle;"`
	KillerIDSoldier sql.NullInt32 `json:"killerIdSoldier" gorm:"index:idx_killevent_killer_id_soldier;default:NULL"`
	KillerSoldier   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDSoldier;"`
	KillerIDVehicle sql.NullInt32 `json:"killerIdVehicle" gorm:"index:idx_killevent_killer_id_vehicle"`
	KillerVehicle   Vehicle       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:KillerIDVehicle;"`

	EventText string  `json:"eventText" gorm:"size:80"`
	Distance  float32 `json:"distance"`
}

func (k *KillEvent) TableName() string {
	return "kill_events"
}

// for medical mods, capture death events (ACE3)
type Ace3DeathEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_deathevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_deathevent_capture_frame;"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_deathevent_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`

	Reason string `json:"reason"`

	LastDamageSourceID sql.NullInt32 `json:"lastDamageSourceId" gorm:"index:idx_deathevent_last_damage_source_id"`
	LastDamageSource   Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:LastDamageSourceID;"`
}

func (a *Ace3DeathEvent) TableName() string {
	return "ace3_death_events"
}

// for medical mods, capture unconscious events (ACE3)
type Ace3UnconsciousEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_unconsciousevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_unconsciousevent_capture_frame;"`
	SoldierID    uint      `json:"soldierId" gorm:"index:idx_unconsciousevent_soldier_id"`
	Soldier      Soldier   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`

	IsAwake bool `json:"isAwake"`
}

func (a *Ace3UnconsciousEvent) TableName() string {
	return "ace3_unconscious_events"
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
	MissionID    uint          `json:"missionId" gorm:"index:idx_chatevent_mission_id"`
	Mission      Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierID    sql.NullInt32 `json:"soldierId" gorm:"index:idx_chatevent_soldier_id;default:NULL"`
	Soldier      Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint          `json:"captureFrame" gorm:"index:idx_chatevent_capture_frame;"`
	Channel      string        `json:"channel" gorm:"size:64"`
	FromName     string        `json:"from" gorm:"size:64"`
	SenderName   string        `json:"name" gorm:"size:64"`
	Message      string        `json:"text"`
	PlayerUID    string        `json:"playerUID" gorm:"size:64; default:NULL; index:idx_chatevent_player_uid"`
}

func (c *ChatEvent) TableName() string {
	return "chat_events"
}

type RadioEvent struct {
	ID           uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time     `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint          `json:"missionId" gorm:"index:idx_radioevent_mission_id"`
	Mission      Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierID    sql.NullInt32 `json:"soldierId" gorm:"index:idx_radioevent_soldier_id;default:NULL"`
	Soldier      Soldier       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:SoldierID;"`
	CaptureFrame uint          `json:"captureFrame" gorm:"index:idx_radioevent_capture_frame;"`

	Radio        string  `json:"radio" gorm:"size:32"`
	RadioType    string  `json:"radioType" gorm:"size:8"`
	StartEnd     string  `json:"startEnd" gorm:"size:8"`
	Channel      int8    `json:"channel"`
	IsAdditional bool    `json:"isAdditional"`
	Frequency    float32 `json:"frequency"`
	Code         string  `json:"code" gorm:"size:32"`
}

func (r *RadioEvent) TableName() string {
	return "radio_events"
}

type ServerFpsEvent struct {
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_serverfpsevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_serverfpsevent_capture_frame;"`
	FpsAverage   float32   `json:"fpsAvg"`
	FpsMin       float32   `json:"fpsMin"`
}

func (s *ServerFpsEvent) TableName() string {
	return "server_fps_events"
}

// Marker represents a map marker
type Marker struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_marker_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_marker_capture_frame;"`

	MarkerName string     `json:"markerName" gorm:"size:128;index:idx_marker_name"`
	Direction  float32    `json:"direction"`
	MarkerType string     `json:"markerType" gorm:"size:64"`
	Text       string     `json:"text" gorm:"size:256"`
	OwnerID    int        `json:"ownerId"`
	Color      string     `json:"color" gorm:"size:32"`
	Size       string     `json:"size" gorm:"size:32"` // stored as "[w,h]"
	Side       string     `json:"side" gorm:"size:16"`
	Position   geom.Point `json:"position"`
	Shape      string     `json:"shape" gorm:"size:32"`
	Alpha      float32    `json:"alpha"`
	Brush      string     `json:"brush" gorm:"size:32"`
	IsDeleted  bool       `json:"isDeleted" gorm:"default:false"`
}

func (*Marker) TableName() string {
	return "markers"
}

// MarkerState tracks marker position changes over time
type MarkerState struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`
	MissionID    uint      `json:"missionId" gorm:"index:idx_markerstate_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	MarkerID     uint      `json:"markerId" gorm:"index:idx_markerstate_marker_id"`
	Marker       Marker    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MarkerID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_markerstate_capture_frame;"`

	Position  geom.Point `json:"position"`
	Direction float32    `json:"direction"`
	Alpha     float32    `json:"alpha"`
}

func (*MarkerState) TableName() string {
	return "marker_states"
}
