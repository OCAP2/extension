package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTableNames(t *testing.T) {
	tests := []struct {
		name     string
		model    interface{ TableName() string }
		expected string
	}{
		{"OcapInfo", &OcapInfo{}, "ocap_infos"},
		{"FrameData", &FrameData{}, "frame_data"},
		{"AfterActionReview", &AfterActionReview{}, "after_action_reviews"},
		{"World", &World{}, "worlds"},
		{"Mission", &Mission{}, "missions"},
		{"Addon", &Addon{}, "addons"},
		{"Soldier", &Soldier{}, "soldiers"},
		{"SoldierState", &SoldierState{}, "soldier_states"},
		{"Vehicle", &Vehicle{}, "vehicles"},
		{"VehicleState", &VehicleState{}, "vehicle_states"},
		{"ProjectileEvent", &ProjectileEvent{}, "projectile_events"},
		{"GeneralEvent", &GeneralEvent{}, "general_events"},
		{"KillEvent", &KillEvent{}, "kill_events"},
		{"Ace3DeathEvent", &Ace3DeathEvent{}, "ace3_death_events"},
		{"Ace3UnconsciousEvent", &Ace3UnconsciousEvent{}, "ace3_unconscious_events"},
		{"ChatEvent", &ChatEvent{}, "chat_events"},
		{"RadioEvent", &RadioEvent{}, "radio_events"},
		{"ServerFpsEvent", &ServerFpsEvent{}, "server_fps_events"},
		{"TimeState", &TimeState{}, "time_states"},
		{"Marker", &Marker{}, "markers"},
		{"MarkerState", &MarkerState{}, "marker_states"},
		{"PlacedObject", &PlacedObject{}, "placed_objects"},
		{"PlacedObjectEvent", &PlacedObjectEvent{}, "placed_object_events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.model.TableName())
		})
	}
}
