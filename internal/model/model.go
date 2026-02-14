package model

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
	MarkerCreates         uint16 `json:"markerCreates"`
	MarkerMoves           uint16 `json:"markerMoves"`
	MarkerDeletes         uint16 `json:"markerDeletes"`
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
	Markers               uint16 `json:"markers"`
	MarkerStates          uint16 `json:"markerStates"`
}

func (b *BufferLengths) TableName() string {
	return "buffer_lengths"
}

////////////////////////
// RETRIEVAL
////////////////////////

// FrameData is the main model for a frame
type FrameData struct {
	ObjectID       uint16         `json:"ocapId"`
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

	Addons        []Addon       `json:"-" gorm:"many2many:mission_addons;"`
	PlayableSlots PlayableSlots `json:"playableSlots" gorm:"embedded;embeddedPrefix:playable_"`
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
// Uses composite primary key (MissionID, ObjectID) - ObjectID is the OCAP-assigned sequential ID
//
// SQF Command: :NEW:SOLDIER:
// Args: [frameNo, ocapId, name, groupId, side, isPlayer, roleDescription, className, displayName, playerUID, squadParams]
type Soldier struct {
	MissionID       uint           `json:"missionId" gorm:"primaryKey;autoIncrement:false"`
	ObjectID        uint16         `json:"ocapId" gorm:"primaryKey;autoIncrement:false"` // OCAP-assigned sequential ID (not Arma netId)
	Mission         Mission        `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	JoinTime        time.Time      `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_soldier_join_time"` // Server time when unit was registered
	JoinFrame       uint           `json:"joinFrame"`                                                             // Frame number when unit was first seen
	OcapType        string         `json:"type" gorm:"size:16;default:man"`                                       // Entity type classification
	UnitName        string         `json:"unitName" gorm:"size:64"`                                               // In-game unit name (from name command)
	GroupID         string         `json:"groupId" gorm:"size:64"`                                                // Group identifier (from groupId command)
	Side            string         `json:"side" gorm:"size:16"`                                                   // Side: WEST, EAST, INDEPENDENT, CIVILIAN
	IsPlayer        bool           `json:"isPlayer" gorm:"default:false"`                                         // Whether unit is player-controlled
	RoleDescription string         `json:"roleDescription" gorm:"size:64"`                                        // Unit role (e.g., "Rifleman", "Medic@Alpha")
	SquadParams     datatypes.JSON `json:"squadParams" gorm:"type:jsonb;default:'[]'"`                            // Squad XML data as JSON
	PlayerUID       string         `json:"playerUID" gorm:"size:64; default:NULL; index:idx_soldier_player_uid"`  // Player UID (empty for AI)
	ClassName       string         `json:"className" gorm:"default:NULL;size:64"`                                 // Config class name (typeOf)
	DisplayName     string         `json:"displayName" gorm:"default:NULL;size:64"`                               // Display name from config
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

// SoldierState tracks soldier state at a point in time
// References Soldier by (MissionID, SoldierObjectID) composite FK
//
// SQF Command: :NEW:SOLDIER:STATE:
// Args: [ocapId, pos, dir, lifeState, inVehicle, name, isPlayer, role, frameNo, hasStableVitals, isDragged, scores, vehicleRole, vehicleOcapId, stance, groupID, side]
type SoldierState struct {
	ID              uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time `json:"time" gorm:"type:timestamptz;"`                                // Server time when state was recorded
	MissionID       uint      `json:"missionId" gorm:"index:idx_soldierstate_mission_id"`
	Mission         Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame    uint      `json:"captureFrame" gorm:"index:idx_capture_frame"`                              // Frame number in recording timeline
	SoldierObjectID uint16    `json:"soldierOcapId" gorm:"index:idx_soldierstate_soldier_ocap_id"`              // OCAP ID of the soldier
	Soldier         Soldier   `gorm:"foreignkey:MissionID,SoldierObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	Position         geom.Point    `json:"position"`                                    // Position ASL (Above Sea Level) as 2D point
	ElevationASL     float32       `json:"elevationASL"`                                // Z coordinate / altitude ASL
	Bearing          uint16        `json:"bearing" gorm:"default:0"`                    // Direction facing (0-360 degrees)
	Lifestate        uint8         `json:"lifestate" gorm:"default:0"`                  // 0=dead, 1=alive, 2=unconscious/incapacitated
	InVehicle         bool          `json:"inVehicle" gorm:"default:false"`      // Whether unit is mounted in a vehicle
	InVehicleObjectID sql.NullInt32 `json:"inVehicleOcapId" gorm:"default:NULL"` // OCAP ID of mounted vehicle (-1/null if not in vehicle)
	VehicleRole      string        `json:"vehicleRole" gorm:"size:64"`                  // Role in vehicle: driver, gunner, commander, cargo, etc.
	UnitName         string        `json:"unitName" gorm:"size:64"`                     // Current unit name (may change, empty if dead)
	IsPlayer         bool          `json:"isPlayer" gorm:"default:false"`              // Whether currently player-controlled
	CurrentRole      string        `json:"currentRole" gorm:"size:64"`                  // Current role/unit type classification
	HasStableVitals  bool          `json:"hasStableVitals" gorm:"default:true"`         // ACE Medical: has stable vitals (true if ACE not present)
	IsDraggedCarried bool          `json:"isDraggedCarried" gorm:"default:false"`       // ACE Medical: is being dragged or carried
	Stance           string        `json:"stance" gorm:"size:64"`                       // Stance: STAND, CROUCH, PRONE, etc.
	GroupID          string        `json:"groupId" gorm:"size:64"`                      // Group name (dynamic, may change mid-mission)
	Side             string        `json:"side" gorm:"size:16"`                         // Side (dynamic, may change mid-mission)
	Scores           SoldierScores `json:"scores" gorm:"embedded;embeddedPrefix:scores_"` // Player score data (only for players)
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
// Uses composite primary key (MissionID, ObjectID) - ObjectID is the OCAP-assigned sequential ID
//
// SQF Command: :NEW:VEHICLE:
// Args: [frameNo, ocapId, vehicleClass, displayName, className, customization]
type Vehicle struct {
	MissionID     uint           `json:"missionId" gorm:"primaryKey;autoIncrement:false"`
	ObjectID      uint16         `json:"ocapId" gorm:"primaryKey;autoIncrement:false"` // OCAP-assigned sequential ID (not Arma netId)
	Mission       Mission        `gorm:"foreignkey:MissionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `json:"deletedAt" gorm:"index"`
	JoinTime      time.Time      `json:"joinTime" gorm:"type:timestamptz;NOT NULL;index:idx_vehicle_join_time"` // Server time when vehicle was registered
	JoinFrame     uint           `json:"joinFrame"`                                                             // Frame number when vehicle was first seen
	OcapType      string         `json:"vehicleClass" gorm:"size:64"`                                           // Vehicle class: car, truck, tank, apc, heli, plane, ship, etc.
	ClassName     string         `json:"className" gorm:"size:64"`                                              // Config class name (typeOf)
	DisplayName   string         `json:"displayName" gorm:"size:64"`                                            // Display name from config
	Customization string         `json:"customization"`                                                         // Vehicle customization data (textures, animations)
}

func (*Vehicle) TableName() string {
	return "vehicles"
}

// VehicleState tracks vehicle state at a point in time
// References Vehicle by (MissionID, VehicleObjectID) composite FK
//
// SQF Command: :NEW:VEHICLE:STATE:
// Args: [ocapId, pos, dir, alive, crew, frameNo, fuel, damage, engineOn, locked, side, vectorDir, vectorUp, turretAz, turretEl]
type VehicleState struct {
	ID              uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time `json:"time" gorm:"type:timestamptz;"`                                            // Server time when state was recorded
	MissionID       uint      `json:"missionId" gorm:"index:idx_vehiclestate_mission_id"`
	Mission         Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame    uint      `json:"captureFrame" gorm:"index:idx_vehiclestate_capture_frame"`                 // Frame number in recording timeline
	VehicleObjectID uint16    `json:"vehicleOcapId" gorm:"index:idx_vehiclestate_vehicle_ocap_id"`              // OCAP ID of the vehicle
	Vehicle         Vehicle   `gorm:"foreignkey:MissionID,VehicleObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	Position        geom.Point `json:"position"`                  // Position ASL as 2D point
	ElevationASL    float32    `json:"elevationASL"`              // Z coordinate / altitude ASL
	Bearing         uint16     `json:"bearing"`                   // Direction facing (0-360 degrees)
	IsAlive         bool       `json:"isAlive"`                   // Whether vehicle is not destroyed
	Crew            string     `json:"crew" gorm:"size:128"`      // Comma-separated OCAP IDs of crew members
	Fuel            float32    `json:"fuel"`                      // Fuel level (0.0-1.0)
	Damage          float32    `json:"damage"`                    // Damage level (0.0-1.0, 1.0 = destroyed)
	Locked          bool       `json:"locked"`                    // Whether vehicle is locked (locked >= 2)
	EngineOn        bool       `json:"engineOn"`                  // Whether engine is running
	Side            string     `json:"side" gorm:"size:16"`       // Side of vehicle owner
	VectorDir       string     `json:"vectorDir" gorm:"size:64"`  // Direction vector [x,y,z] as string
	VectorUp        string     `json:"vectorUp" gorm:"size:64"`   // Up vector [x,y,z] as string
	TurretAzimuth   float32    `json:"turretAzimuth"`             // Main turret horizontal rotation (degrees)
	TurretElevation float32    `json:"turretElevation"`           // Main turret vertical angle (degrees)
}

func (*VehicleState) TableName() string {
	return "vehicle_states"
}

// FiredEvent represents a weapon being fired (legacy bullet tracking)
// References Soldier by (MissionID, SoldierObjectID) composite FK
//
// SQF Command: :FIRED:
// Args: [firerOcapId, frameNo, endPos, startPos, weaponDisplay, magazineDisplay, fireMode]
type FiredEvent struct {
	ID              uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time `json:"time" gorm:"type:timestamptz;"`                                            // Server time when fired
	MissionID       uint      `json:"missionId" gorm:"index:idx_firedevent_mission_id"`
	Mission         Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierObjectID uint16    `json:"soldierOcapId" gorm:"index:idx_firedevent_soldier_ocap_id"`                // OCAP ID of the firer
	Soldier         Soldier   `gorm:"foreignkey:MissionID,SoldierObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CaptureFrame    uint      `json:"captureFrame" gorm:"index:idx_firedevent_capture_frame;"`                  // Frame number when fired
	Weapon          string    `json:"weapon" gorm:"size:64"`                                                    // Weapon/muzzle display name
	Magazine        string    `json:"magazine" gorm:"size:64"`                                                  // Magazine display name
	FiringMode      string    `json:"mode" gorm:"size:64"`                                                      // Fire mode: single, burst, auto

	StartPosition     geom.Point `json:"startPos"`  // Bullet origin position ASL (firer position)
	StartElevationASL float32    `json:"startElev"` // Bullet origin Z coordinate
	EndPosition       geom.Point `json:"endPos"`    // Bullet destination/impact position ASL
	EndElevationASL   float32    `json:"endElev"`   // Bullet destination Z coordinate
}

func (*FiredEvent) TableName() string {
	return "fired_events"
}

// ProjectileEvent represents a weapon being fired and its full lifetime tracking
// References Soldier by ObjectID for Firer and ActualFirer (remote controller)
//
// SQF Command: :PROJECTILE:
// Args: [firedFrame, firedTime, firerID, vehicleID, vehicleRole, remoteControllerID,
//
//	weapon, weaponDisplay, muzzle, muzzleDisplay, magazine, magazineDisplay,
//	ammo, fireMode, positions, initialVelocity, hitParts, sim, isSub, magazineIcon]
type ProjectileEvent struct {
	ID                  uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time                time.Time     `json:"firedTime" gorm:"type:timestamptz;"`                                          // Server time when fired (from diag_tickTime)
	MissionID           uint          `json:"missionId" gorm:"index:idx_projectile_mission_id"`
	Mission             Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	FirerObjectID       uint16        `json:"firerOcapId" gorm:"index:idx_projectile_firer_ocap_id"`                        // OCAP ID of unit that fired
	Firer               Soldier       `gorm:"foreignkey:MissionID,FirerObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ActualFirerObjectID uint16        `json:"actualFirerOcapId" gorm:"index:idx_projectile_actual_firer_ocap_id"`           // OCAP ID of actual controller (for remote-controlled units)
	ActualFirer         Soldier       `gorm:"foreignkey:MissionID,ActualFirerObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	VehicleRole         string        `json:"vehicleRole" gorm:"size:32"`                                                   // Role in vehicle if fired from one: driver, gunner, etc.
	VehicleObjectID     sql.NullInt32 `json:"vehicleOcapId,omitempty" gorm:"index:idx_projectile_vehicle_ocap_id"`          // OCAP ID of vehicle if fired from one (-1/null if not)
	CaptureFrame        uint          `json:"firedFrame" gorm:"index:idx_projectile_capture_frame;"`                        // Frame number when fired

	Positions geom.Geometry `json:"-"` // LineStringZM of projectile positions over time [x,y,z,tickTime]

	// Weapon/ammo data
	InitialVelocity string `json:"initialVelocity" gorm:"size:64"` // Initial velocity vector "vx,vy,vz"
	Weapon          string `json:"weapon" gorm:"size:64"`          // Weapon class name
	WeaponDisplay   string `json:"weaponDisplay" gorm:"size:64"`   // Weapon display name
	Magazine        string `json:"magazine" gorm:"size:64"`        // Magazine class name
	MagazineDisplay string `json:"magazineDisplay" gorm:"size:64"` // Magazine display name
	Muzzle          string `json:"muzzle" gorm:"size:64"`          // Muzzle class name
	MuzzleDisplay   string `json:"muzzleDisplay" gorm:"size:64"`   // Muzzle display name
	Ammo            string `json:"ammo" gorm:"size:64"`            // Ammo class name
	Mode            string `json:"mode" gorm:"size:32"`            // Fire mode: single, burst, auto

	// Simulation data
	SimulationType string `json:"simulationType" gorm:"size:32"` // Ammo simulation: shotBullet, shotShell, shotRocket, shotMissile, etc.
	IsSubmunition  bool   `json:"isSubmunition"`                 // Whether this is a submunition (from cluster/split ammo)
	MagazineIcon   string `json:"magazineIcon" gorm:"size:128"`  // Path to magazine icon texture

	// Projectile hits
	HitSoldiers []ProjectileHitsSoldier `json:"hitsSoldiers" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	HitVehicles []ProjectileHitsVehicle `json:"hitsVehicles" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (p *ProjectileEvent) TableName() string {
	return "projectile_events"
}

// ProjectileHitsSoldier records when a projectile hits a soldier
// Part of hitParts array in :PROJECTILE: command: [hitOcapId, component, "x,y,z", frameNo]
type ProjectileHitsSoldier struct {
	ID                uint            `json:"id" gorm:"primarykey;autoIncrement;"`
	ProjectileEventID uint            `json:"projectileEventId" gorm:"index:idx_projectile_hit_soldier_projectile_event_id"`
	ProjectileEvent   ProjectileEvent `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ProjectileEventID;"`
	MissionID         uint            `json:"missionId"`
	SoldierObjectID   uint16          `json:"soldierOcapId" gorm:"index:idx_projectile_hit_soldier_ocap_id"` // OCAP ID of hit soldier
	Soldier           Soldier         `gorm:"foreignkey:MissionID,SoldierObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CaptureFrame      uint            `json:"captureFrame" gorm:"index:idx_projectile_hit_soldier_capture_frame;"` // Frame when hit occurred
	Position          geom.Point      `json:"position"`                                                            // Impact position ASL
	ComponentsHit     datatypes.JSON  `json:"componentsHit"`                                                       // Body parts hit as JSON array
}

// ProjectileHitsVehicle records when a projectile hits a vehicle
// Part of hitParts array in :PROJECTILE: command: [hitOcapId, component, "x,y,z", frameNo]
type ProjectileHitsVehicle struct {
	ID                uint            `json:"id" gorm:"primarykey;autoIncrement;"`
	ProjectileEventID uint            `json:"projectileEventId" gorm:"index:idx_projectile_hit_vehicle_projectile_event_id"`
	ProjectileEvent   ProjectileEvent `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:ProjectileEventID;"`
	MissionID         uint            `json:"missionId"`
	VehicleObjectID   uint16          `json:"vehicleOcapId" gorm:"index:idx_projectile_hit_vehicle_ocap_id"` // OCAP ID of hit vehicle
	Vehicle           Vehicle         `gorm:"foreignkey:MissionID,VehicleObjectID;references:MissionID,ObjectID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CaptureFrame      uint            `json:"captureFrame" gorm:"index:idx_projectile_hit_vehicle_capture_frame;"` // Frame when hit occurred
	Position          geom.Point      `json:"position"`                                                            // Impact position ASL
	ComponentsHit     datatypes.JSON  `json:"componentsHit"`                                                       // Vehicle components hit as JSON array
}

// GeneralEvent is a generic event for player connections, mission end, custom events
//
// SQF Command: :EVENT:
// Args: [frameNo, eventType, message, extraDataJSON]
// Common eventTypes: "connected", "disconnected", "endMission"
type GeneralEvent struct {
	ID           uint           `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time      `json:"time" gorm:"type:timestamptz;"`                              // Server time when event occurred
	MissionID    uint           `json:"missionId" gorm:"index:idx_generalevent_mission_id"`
	Mission      Mission        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint           `json:"captureFrame" gorm:"index:idx_generalevent_capture_frame;"` // Frame number when event occurred
	Name         string         `json:"name" gorm:"size:64"`                                       // Event type: connected, disconnected, endMission, custom
	Message      string         `json:"message"`                                                   // Event message (e.g., player name)
	ExtraData    datatypes.JSON `json:"extraData" gorm:"type:jsonb;default:'{}'"`                  // Additional JSON data (e.g., playerUid)
}

func (g *GeneralEvent) TableName() string {
	return "general_events"
}

// HitEvent represents an entity being hit by a projectile or explosion
// Stores ObjectIDs directly - victim/shooter could be soldier or vehicle
//
// SQF Command: :HIT:
// Args: [frameNo, victimOcapId, shooterOcapId, weaponText, distance]
type HitEvent struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`                              // Server time when hit occurred
	MissionID    uint      `json:"missionId" gorm:"index:idx_hitevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_hitevent_capture_frame;"`      // Frame number when hit occurred

	// Victim OCAP ID - one of these will be set based on entity type
	VictimSoldierObjectID  sql.NullInt32 `json:"victimSoldierOcapId" gorm:"index:idx_hitevent_victim_soldier_ocap;default:NULL"`  // OCAP ID if victim is soldier
	VictimVehicleObjectID  sql.NullInt32 `json:"victimVehicleOcapId" gorm:"index:idx_hitevent_victim_vehicle_ocap;default:NULL"`  // OCAP ID if victim is vehicle
	// Shooter OCAP ID - one of these will be set based on entity type
	ShooterSoldierObjectID sql.NullInt32 `json:"shooterSoldierOcapId" gorm:"index:idx_hitevent_shooter_soldier_ocap;default:NULL"` // OCAP ID if shooter is soldier
	ShooterVehicleObjectID sql.NullInt32 `json:"shooterVehicleOcapId" gorm:"index:idx_hitevent_shooter_vehicle_ocap;default:NULL"` // OCAP ID if shooter is vehicle

	EventText string  `json:"eventText" gorm:"size:80"` // Weapon/cause description (from getEventWeaponText)
	Distance  float32 `json:"distance"`                 // Distance between shooter and victim in meters
}

func (h *HitEvent) TableName() string {
	return "hit_events"
}

// KillEvent represents an entity being killed/destroyed
// Stores ObjectIDs directly - victim/killer could be soldier or vehicle
//
// SQF Command: :KILL:
// Args: [frameNo, victimOcapId, killerOcapId, weaponText, distance]
type KillEvent struct {
	ID   uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time time.Time `json:"time" gorm:"type:timestamptz;"` // Server time when kill occurred

	MissionID    uint    `json:"missionId" gorm:"index:idx_killevent_mission_id"`
	Mission      Mission `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint    `json:"captureFrame" gorm:"index:idx_killevent_capture_frame;"` // Frame number when kill occurred

	// Victim OCAP ID - one of these will be set based on entity type
	VictimSoldierObjectID sql.NullInt32 `json:"victimSoldierOcapId" gorm:"index:idx_killevent_victim_soldier_ocap;default:NULL"` // OCAP ID if victim is soldier
	VictimVehicleObjectID sql.NullInt32 `json:"victimVehicleOcapId" gorm:"index:idx_killevent_victim_vehicle_ocap;default:NULL"` // OCAP ID if victim is vehicle
	// Killer OCAP ID - one of these will be set based on entity type
	KillerSoldierObjectID sql.NullInt32 `json:"killerSoldierOcapId" gorm:"index:idx_killevent_killer_soldier_ocap;default:NULL"` // OCAP ID if killer is soldier
	KillerVehicleObjectID sql.NullInt32 `json:"killerVehicleOcapId" gorm:"index:idx_killevent_killer_vehicle_ocap;default:NULL"` // OCAP ID if killer is vehicle

	EventText string  `json:"eventText" gorm:"size:80"` // Weapon/cause description
	Distance  float32 `json:"distance"`                 // Distance between killer and victim in meters
}

func (k *KillEvent) TableName() string {
	return "kill_events"
}

// Ace3DeathEvent captures death events with medical cause from ACE3 mod
// Stores ObjectIDs directly
//
// SQF Command: :ACE3:DEATH:
// Args: [frameNo, victimOcapId, causeOfDeath, lastDamageSourceOcapId]
type Ace3DeathEvent struct {
	ID              uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time `json:"time" gorm:"type:timestamptz;"`                               // Server time when death occurred
	MissionID       uint      `json:"missionId" gorm:"index:idx_deathevent_mission_id"`
	Mission         Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame    uint      `json:"captureFrame" gorm:"index:idx_deathevent_capture_frame;"`     // Frame number when death occurred
	SoldierObjectID uint16    `json:"soldierOcapId" gorm:"index:idx_deathevent_soldier_ocap_id"`   // OCAP ID of dead soldier

	Reason string `json:"reason"` // ACE3 cause of death (from ace_medical_causeOfDeath variable)

	LastDamageSourceObjectID sql.NullInt32 `json:"lastDamageSourceOcapId" gorm:"index:idx_deathevent_last_damage_source_ocap"` // OCAP ID of last damage source (-1/null if none)
}

func (a *Ace3DeathEvent) TableName() string {
	return "ace3_death_events"
}

// Ace3UnconsciousEvent captures unconscious state changes from ACE3 mod
// Stores ObjectID directly
//
// SQF Command: :ACE3:UNCONSCIOUS:
// Args: [frameNo, soldierOcapId, isUnconscious]
type Ace3UnconsciousEvent struct {
	ID              uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time `json:"time" gorm:"type:timestamptz;"`                                       // Server time when state changed
	MissionID       uint      `json:"missionId" gorm:"index:idx_unconsciousevent_mission_id"`
	Mission         Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame    uint      `json:"captureFrame" gorm:"index:idx_unconsciousevent_capture_frame;"`       // Frame number when state changed
	SoldierObjectID uint16    `json:"soldierOcapId" gorm:"index:idx_unconsciousevent_soldier_ocap_id"`     // OCAP ID of soldier

	IsUnconscious bool `json:"isUnconscious"` // true = went unconscious, false = regained consciousness
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

// ChatEvent records chat messages
//
// SQF Command: :CHAT:
// Args: [frameNo, senderOcapId, channel, from, name, text, playerUID]
type ChatEvent struct {
	ID              uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time     `json:"time" gorm:"type:timestamptz;"`                                           // Server time when message sent
	MissionID       uint          `json:"missionId" gorm:"index:idx_chatevent_mission_id"`
	Mission         Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierObjectID sql.NullInt32 `json:"soldierOcapId" gorm:"index:idx_chatevent_soldier_ocap_id;default:NULL"`   // OCAP ID of sender (-1/null if system)
	CaptureFrame    uint          `json:"captureFrame" gorm:"index:idx_chatevent_capture_frame;"`                  // Frame number when message sent
	Channel         string        `json:"channel" gorm:"size:64"`                                                  // Channel name: Global, Side, Command, Group, Vehicle, Direct, Custom, System
	FromName        string        `json:"from" gorm:"size:64"`                                                     // Formatted sender identifier (as shown in game)
	SenderName      string        `json:"name" gorm:"size:64"`                                                     // Actual sender name
	Message         string        `json:"text"`                                                                    // Message content
	PlayerUID       string        `json:"playerUID" gorm:"size:64; default:NULL; index:idx_chatevent_player_uid"`  // Player UID of sender
}

func (c *ChatEvent) TableName() string {
	return "chat_events"
}

// RadioEvent records TFAR radio transmissions
//
// SQF Command: :RADIO:
// Args: [frameNo, senderOcapId, radio, radioType, startEnd, channel, isAdditional, frequency, code]
type RadioEvent struct {
	ID              uint          `json:"id" gorm:"primarykey;autoIncrement;"`
	Time            time.Time     `json:"time" gorm:"type:timestamptz;"`                                           // Server time when transmission occurred
	MissionID       uint          `json:"missionId" gorm:"index:idx_radioevent_mission_id"`
	Mission         Mission       `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	SoldierObjectID sql.NullInt32 `json:"soldierOcapId" gorm:"index:idx_radioevent_soldier_ocap_id;default:NULL"`  // OCAP ID of transmitting soldier
	CaptureFrame    uint          `json:"captureFrame" gorm:"index:idx_radioevent_capture_frame;"`                 // Frame number when transmission occurred

	Radio        string  `json:"radio" gorm:"size:32"`    // Radio device name/identifier
	RadioType    string  `json:"radioType" gorm:"size:8"` // Radio type: SW (Short Wave) or LR (Long Range)
	StartEnd     string  `json:"startEnd" gorm:"size:8"`  // Transmission state: Start or Stop
	Channel      int8    `json:"channel"`                 // Radio channel number (1-indexed)
	IsAdditional bool    `json:"isAdditional"`            // Whether using additional/alternate channel
	Frequency    float32 `json:"frequency"`               // Radio frequency
	Code         string  `json:"code" gorm:"size:32"`     // Radio encryption code
}

func (r *RadioEvent) TableName() string {
	return "radio_events"
}

// ServerFpsEvent records server performance metrics
//
// SQF Command: :FPS:
// Args: [frameNo, currentFps, minFps]
type ServerFpsEvent struct {
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`                              // Server time when measurement taken
	MissionID    uint      `json:"missionId" gorm:"index:idx_serverfpsevent_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_serverfpsevent_capture_frame;"` // Frame number when measurement taken
	FpsAverage   float32   `json:"fpsAvg"`                                                     // Current server FPS (from diag_fps)
	FpsMin       float32   `json:"fpsMin"`                                                     // Minimum FPS in sample period (from diag_fpsmin)
}

func (s *ServerFpsEvent) TableName() string {
	return "server_fps_events"
}

// TimeState represents mission time synchronization data
//
// SQF Command: :NEW:TIME:STATE:
// Args: [frameNo, systemTimeUTC, missionDateTime, timeMultiplier, missionTime]
type TimeState struct {
	Time           time.Time `json:"time" gorm:"type:timestamptz;"`                              // Server time when recorded
	MissionID      uint      `json:"missionId" gorm:"index:idx_timestate_mission_id"`
	Mission        Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame   uint      `json:"captureFrame" gorm:"index:idx_timestate_capture_frame;"`     // Frame number when recorded
	SystemTimeUTC  string    `json:"systemTimeUtc" gorm:"size:64"`                               // Real-world system time (ISO 8601: YYYY-MM-DDTHH:MM:SS.mmm)
	MissionDate    string    `json:"missionDate" gorm:"size:64"`                                 // In-game mission date/time (ISO 8601: YYYY-MM-DDTHH:MM:00)
	TimeMultiplier float32   `json:"timeMultiplier"`                                             // Mission time acceleration multiplier
	MissionTime    float32   `json:"missionTime"`                                                // Seconds elapsed since mission start
}

func (t *TimeState) TableName() string {
	return "time_states"
}

// Marker represents a map marker
//
// SQF Command: :NEW:MARKER:
// Args: [markerName, direction, type, text, frameNo, -1, ownerOcapId, color, size, side, position, shape, alpha, brush]
type Marker struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`                               // Server time when marker created
	MissionID    uint      `json:"missionId" gorm:"index:idx_marker_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_marker_capture_frame;"`         // Frame number when marker created

	MarkerName string          `json:"markerName" gorm:"size:128;index:idx_marker_name"` // Unique marker identifier
	Direction  float32         `json:"direction"`                                        // Marker rotation (0-360 degrees)
	MarkerType string          `json:"markerType" gorm:"size:64"`                        // Marker type (e.g., "mil_dot", "mil_objective")
	Text       string          `json:"text" gorm:"size:256"`                             // Marker label text
	OwnerID    int             `json:"ownerId"`                                          // OCAP ID of marker owner (-1 for global markers)
	Color      string          `json:"color" gorm:"size:32"`                             // Marker color
	Size       string          `json:"size" gorm:"size:32"`                              // Marker size as "[width,height]"
	Side       string          `json:"side" gorm:"size:16"`                              // Side restriction (-1 for global, or side enum)
	Position   geom.Point      `json:"position"`                                         // Marker position ASL (for non-polyline markers)
	Polyline   geom.LineString `json:"polyline"`                                         // Polyline positions (for POLYLINE shape markers)
	Shape      string          `json:"shape" gorm:"size:32"`                             // Shape: ICON, RECTANGLE, ELLIPSE, POLYLINE
	Alpha      float32         `json:"alpha"`                                            // Marker opacity (0.0-1.0)
	Brush      string          `json:"brush" gorm:"size:32"`                             // Fill pattern for area markers
	IsDeleted  bool            `json:"isDeleted" gorm:"default:false"`                   // Whether marker has been deleted
}

func (*Marker) TableName() string {
	return "markers"
}

// MarkerState tracks marker position/property changes over time
//
// SQF Command: :NEW:MARKER:STATE:
// Args: [markerName, frameNo, position, direction, alpha]
type MarkerState struct {
	ID           uint      `json:"id" gorm:"primarykey;autoIncrement;"`
	Time         time.Time `json:"time" gorm:"type:timestamptz;"`                               // Server time when state recorded
	MissionID    uint      `json:"missionId" gorm:"index:idx_markerstate_mission_id"`
	Mission      Mission   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MissionID;"`
	MarkerID     uint      `json:"markerId" gorm:"index:idx_markerstate_marker_id"`             // Database ID of parent Marker
	Marker       Marker    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignkey:MarkerID;"`
	CaptureFrame uint      `json:"captureFrame" gorm:"index:idx_markerstate_capture_frame;"`    // Frame number when state recorded

	Position  geom.Point `json:"position"`  // Current marker position ASL
	Direction float32    `json:"direction"` // Current marker rotation (0-360 degrees)
	Alpha     float32    `json:"alpha"`     // Current marker opacity (0.0-1.0, 0 = deleted)
}

func (*MarkerState) TableName() string {
	return "marker_states"
}
