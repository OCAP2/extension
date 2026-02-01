// internal/model/core/types.go
package core

// Position3D represents a 3D coordinate without GIS dependencies
type Position3D struct {
	X float64 `json:"x"` // easting
	Y float64 `json:"y"` // northing
	Z float64 `json:"z"` // elevation ASL
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

// PlayableSlots shows counts of playable slots by side
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
