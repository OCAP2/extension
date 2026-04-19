package v1

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/OCAP2/extension/v5/internal/util"
	"github.com/OCAP2/extension/v5/pkg/core"
)

// Stream writes the v1 export JSON directly to w without holding the
// full Export struct in memory. Peak additional memory is bounded by
// the largest single entity's gap-filled positions plus the aggregate
// events/markers slices, which are typically orders of magnitude
// smaller than the sum of all entity positions.
//
// Output is JSON-equivalent to json.NewEncoder(w).Encode(Build(data)).
// The entities array is indexed by ID so map iteration order does not
// affect final output.
func Stream(w io.Writer, data *MissionData) error {
	bw := bufio.NewWriterSize(w, 64*1024)

	maxFrame := computeMaxFrame(data)
	maxEntityID, hasEntities := computeMaxEntityID(data)
	times := buildTimes(data)
	events, markers, firelines := buildAggregates(data)

	// Sort events by frame so consumers can rely on chronological order.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i][0].(int) < events[j][0].(int)
	})

	if err := bw.WriteByte('{'); err != nil {
		return err
	}

	// Scalar top-level fields, in the same order as the Export struct.
	if err := writeField(bw, false, "addonVersion", data.Mission.AddonVersion); err != nil {
		return err
	}
	if err := writeField(bw, true, "extensionVersion", data.Mission.ExtensionVersion); err != nil {
		return err
	}
	if err := writeField(bw, true, "extensionBuild", data.Mission.ExtensionBuild); err != nil {
		return err
	}
	if err := writeField(bw, true, "missionName", data.Mission.MissionName); err != nil {
		return err
	}
	if err := writeField(bw, true, "missionAuthor", data.Mission.Author); err != nil {
		return err
	}
	if err := writeField(bw, true, "worldName", data.World.WorldName); err != nil {
		return err
	}
	if err := writeField(bw, true, "endFrame", frameToV1(maxFrame)); err != nil {
		return err
	}
	if err := writeField(bw, true, "captureDelay", data.Mission.CaptureDelay); err != nil {
		return err
	}
	if err := writeField(bw, true, "tags", data.Mission.Tag); err != nil {
		return err
	}
	if err := writeField(bw, true, "times", times); err != nil {
		return err
	}

	// entities array, streamed one entry at a time. Each entity's
	// gap-filled positions slice becomes eligible for GC after its
	// iteration completes.
	if _, err := bw.WriteString(`,"entities":[`); err != nil {
		return err
	}
	if hasEntities {
		for id := uint16(0); ; id++ {
			if id > 0 {
				if err := bw.WriteByte(','); err != nil {
					return err
				}
			}
			if err := writeEntityForID(bw, id, data, maxFrame, firelines[id]); err != nil {
				return err
			}
			if id == maxEntityID {
				break
			}
		}
	}
	if err := bw.WriteByte(']'); err != nil {
		return err
	}

	if err := writeField(bw, true, "events", events); err != nil {
		return err
	}
	if err := writeField(bw, true, "Markers", markers); err != nil {
		return err
	}

	if err := bw.WriteByte('}'); err != nil {
		return err
	}
	return bw.Flush()
}

// writeField marshals a single JSON field into the writer. When
// leading is true, a comma is emitted before the field.
func writeField(bw *bufio.Writer, leading bool, key string, value any) error {
	if leading {
		if err := bw.WriteByte(','); err != nil {
			return err
		}
	}
	keyBytes, err := json.Marshal(key)
	if err != nil {
		return err
	}
	if _, err := bw.Write(keyBytes); err != nil {
		return err
	}
	if err := bw.WriteByte(':'); err != nil {
		return err
	}
	valBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = bw.Write(valBytes)
	return err
}

// writeEntityForID writes the JSON for the entity at the given ID. If
// no soldier or vehicle owns that ID, a zero-valued Entity is written
// to preserve index=ID parity with Build(), which uses make([]Entity, ...)
// and leaves unfilled slots as the zero value.
//
// The entity struct is scoped to this call so its gap-filled Positions
// slice becomes eligible for GC as soon as this function returns.
func writeEntityForID(bw *bufio.Writer, id uint16, data *MissionData, maxFrame core.Frame, extraFirelines [][]any) error {
	if soldier, ok := data.Soldiers[id]; ok {
		ent := buildSoldierEntity(soldier, maxFrame, extraFirelines)
		return writeJSON(bw, ent)
	}
	if vehicle, ok := data.Vehicles[id]; ok {
		ent := buildVehicleEntity(vehicle, maxFrame)
		return writeJSON(bw, ent)
	}
	return writeJSON(bw, Entity{})
}

// writeJSON marshals v and writes the bytes to bw.
func writeJSON(bw *bufio.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = bw.Write(b)
	return err
}

// computeMaxFrame returns the maximum capture frame across all entity
// states. Falls back to 0 (FrameForever) if no state was recorded.
func computeMaxFrame(data *MissionData) core.Frame {
	var maxFrame core.Frame
	for _, record := range data.Soldiers {
		for _, state := range record.States {
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}
	}
	for _, record := range data.Vehicles {
		for _, state := range record.States {
			if state.CaptureFrame > maxFrame {
				maxFrame = state.CaptureFrame
			}
		}
	}
	return maxFrame
}

// computeMaxEntityID returns the largest soldier/vehicle ID so the
// entities array can be sized to maxID+1 and the frontend can use
// array[ID] lookups. The bool is false when no entities exist.
func computeMaxEntityID(data *MissionData) (uint16, bool) {
	var maxID uint16
	hasEntities := len(data.Soldiers) > 0 || len(data.Vehicles) > 0
	for _, record := range data.Soldiers {
		if record.Soldier.ID > maxID {
			maxID = record.Soldier.ID
		}
	}
	for _, record := range data.Vehicles {
		if record.Vehicle.ID > maxID {
			maxID = record.Vehicle.ID
		}
	}
	return maxID, hasEntities
}

// buildTimes converts core TimeStates into v1 Time entries.
func buildTimes(data *MissionData) []Time {
	times := make([]Time, 0, len(data.TimeStates))
	for _, ts := range data.TimeStates {
		times = append(times, Time{
			Date:           ts.MissionDate,
			FrameNum:       frameToV1(ts.CaptureFrame),
			SystemTimeUTC:  ts.SystemTimeUTC,
			Time:           ts.MissionTime,
			TimeMultiplier: ts.TimeMultiplier,
		})
	}
	return times
}

// buildAggregates returns every non-entity value the Export needs:
// events (unsorted — caller sorts), markers (static + placed + projectile
// trajectories), and the per-firer firelines consumed by soldier
// entities via their FramesFired.
func buildAggregates(data *MissionData) (events [][]any, markers [][]any, firelines map[uint16][][]any) {
	events = make([][]any, 0)
	markers = make([][]any, 0)
	firelines = make(map[uint16][][]any)

	// General events: [frameNum, "type", message]
	for _, evt := range data.GeneralEvents {
		var message any = evt.Message
		if len(evt.Message) > 0 && (evt.Message[0] == '[' || evt.Message[0] == '{') {
			var parsed any
			if err := json.Unmarshal([]byte(evt.Message), &parsed); err == nil {
				message = parsed
			}
		}
		events = append(events, []any{frameToV1(evt.CaptureFrame), evt.Name, message})
	}

	// Sector events: [frameNum, "captured"|"contested", [...]]
	for _, evt := range data.SectorEvents {
		events = append(events, []any{
			frameToV1(evt.CaptureFrame),
			evt.Name,
			[]any{evt.ObjectType, evt.UnitName, evt.Side, []float64{evt.PosX, evt.PosY, evt.PosZ}},
		})
	}

	// End-mission events: [frameNum, "endMission", [side, message]]
	for _, evt := range data.EndMissionEvents {
		events = append(events, []any{
			frameToV1(evt.CaptureFrame),
			"endMission",
			[]any{evt.Side, evt.Message},
		})
	}

	// Hit events: [frameNum, "hit", victimId, [causedById, weapon], distance]
	for _, evt := range data.HitEvents {
		var victimID uint
		if evt.VictimVehicleID != nil {
			victimID = *evt.VictimVehicleID
		} else if evt.VictimSoldierID != nil {
			victimID = *evt.VictimSoldierID
		}
		var sourceID uint
		if evt.ShooterVehicleID != nil {
			sourceID = *evt.ShooterVehicleID
		} else if evt.ShooterSoldierID != nil {
			sourceID = *evt.ShooterSoldierID
		}
		events = append(events, []any{
			frameToV1(evt.CaptureFrame),
			"hit",
			victimID,
			[]any{sourceID, evt.EventText},
			evt.Distance,
		})
	}

	// Kill events: [frameNum, "killed", victimId, [causedById, weapon], distance]
	for _, evt := range data.KillEvents {
		var victimID uint
		if evt.VictimVehicleID != nil {
			victimID = *evt.VictimVehicleID
		} else if evt.VictimSoldierID != nil {
			victimID = *evt.VictimSoldierID
		}
		var killerID uint
		if evt.KillerVehicleID != nil {
			killerID = *evt.KillerVehicleID
		} else if evt.KillerSoldierID != nil {
			killerID = *evt.KillerSoldierID
		}
		events = append(events, []any{
			frameToV1(evt.CaptureFrame),
			"killed",
			victimID,
			[]any{killerID, evt.EventText},
			evt.Distance,
		})
	}

	// Static markers.
	for _, record := range data.Markers {
		markerColor := strings.TrimPrefix(record.Marker.Color, "#")
		posArray := make([][]any, 0)

		if record.Marker.Shape == "POLYLINE" {
			coords := make([][]float64, len(record.Marker.Polyline))
			for i, pt := range record.Marker.Polyline {
				coords[i] = []float64{pt.X, pt.Y}
			}
			posArray = append(posArray, []any{
				frameToV1(record.Marker.CaptureFrame),
				coords,
				record.Marker.Direction,
				record.Marker.Alpha,
				record.Marker.Text,
				markerColor,
				parseMarkerSize(record.Marker.Size),
				record.Marker.MarkerType,
				record.Marker.Brush,
				record.Marker.Shape,
			})
		} else {
			posArray = append(posArray, []any{
				frameToV1(record.Marker.CaptureFrame),
				[]float64{record.Marker.Position.X, record.Marker.Position.Y, record.Marker.Position.Z},
				record.Marker.Direction,
				record.Marker.Alpha,
				record.Marker.Text,
				markerColor,
				parseMarkerSize(record.Marker.Size),
				record.Marker.MarkerType,
				record.Marker.Brush,
				record.Marker.Shape,
			})

			for _, state := range record.States {
				posArray = append(posArray, []any{
					frameToV1(state.CaptureFrame),
					[]float64{state.Position.X, state.Position.Y, state.Position.Z},
					state.Direction,
					state.Alpha,
					state.Text,
					strings.TrimPrefix(state.Color, "#"),
					parseMarkerSize(state.Size),
					state.MarkerType,
					state.Brush,
					state.Shape,
				})
			}
		}

		markers = append(markers, []any{
			record.Marker.MarkerType,
			record.Marker.Text,
			frameToV1(record.Marker.CaptureFrame),
			frameToV1(record.Marker.EndFrame),
			record.Marker.OwnerID,
			markerColor,
			sideToIndex(record.Marker.Side),
			posArray,
			parseMarkerSize(record.Marker.Size),
			record.Marker.Shape,
			record.Marker.Brush,
		})
	}

	// Placed objects become markers, and their "hit" events become event rows.
	for _, record := range data.PlacedObjects {
		iconFilename := extractFilename(record.PlacedObject.MagazineIcon)
		var markerType string
		if iconFilename != "" {
			markerType = "magIcons/" + iconFilename
		} else {
			markerType = "Minefield"
		}

		placedEndFrame := -1
		for _, evt := range record.Events {
			if evt.EventType == "detonated" || evt.EventType == "deleted" {
				placedEndFrame = frameToV1(evt.CaptureFrame)
				break
			}
		}

		posArray := [][]any{
			{
				frameToV1(record.PlacedObject.JoinFrame),
				[]float64{record.PlacedObject.Position.X, record.PlacedObject.Position.Y, record.PlacedObject.Position.Z},
				0,
				1.0,
			},
		}

		markers = append(markers, []any{
			markerType,
			record.PlacedObject.DisplayName,
			frameToV1(record.PlacedObject.JoinFrame),
			placedEndFrame,
			int(record.PlacedObject.OwnerID),
			"D96600",
			-1,
			posArray,
			[]float64{1, 1},
			"ICON",
			"Solid",
		})

		for _, evt := range record.Events {
			if evt.EventType == "hit" && evt.HitEntityID != nil {
				dx := record.PlacedObject.Position.X - evt.Position.X
				dy := record.PlacedObject.Position.Y - evt.Position.Y
				dist := float32(math.Sqrt(dx*dx + dy*dy))
				events = append(events, []any{
					frameToV1(evt.CaptureFrame),
					"hit",
					uint(*evt.HitEntityID),
					[]any{uint(record.PlacedObject.OwnerID), record.PlacedObject.DisplayName},
					dist,
				})
			}
		}
	}

	// Projectile events: bullets become per-firer firelines; non-bullet
	// projectiles become markers; both produce hit events.
	for _, pe := range data.ProjectileEvents {
		if !isProjectileMarker(pe.SimulationType) {
			if len(pe.Trajectory) >= 2 {
				endPt := pe.Trajectory[len(pe.Trajectory)-1]
				ff := []any{
					frameToV1(pe.CaptureFrame),
					[]float64{endPt.Position.X, endPt.Position.Y, endPt.Position.Z},
				}
				firelines[pe.FirerObjectID] = append(firelines[pe.FirerObjectID], ff)
			}
		} else {
			iconFilename := extractFilename(pe.MagazineIcon)
			var markerType, color string
			if iconFilename != "" {
				markerType = "magIcons/" + iconFilename
				color = "ColorWhite"
			} else {
				markerType = "mil_triangle"
				color = "ColorRed"
			}

			var text string
			switch {
			case pe.VehicleObjectID != nil && *pe.VehicleObjectID != pe.FirerObjectID:
				vehicleName := ""
				if vr, ok := data.Vehicles[*pe.VehicleObjectID]; ok {
					vehicleName = vr.Vehicle.DisplayName
				}
				text = fmt.Sprintf("%s %s - %s", vehicleName, pe.MuzzleDisplay, pe.MagazineDisplay)
			case pe.SimulationType == "shotGrenade":
				text = pe.MagazineDisplay
			default:
				text = fmt.Sprintf("%s - %s", pe.MuzzleDisplay, pe.MagazineDisplay)
			}

			posArray := make([][]any, 0, len(pe.Trajectory))
			for _, tp := range pe.Trajectory {
				posArray = append(posArray, []any{
					frameToV1(tp.FrameNum),
					[]float64{tp.Position.X, tp.Position.Y, tp.Position.Z},
					0,
					1.0,
				})
			}

			projEndFrame := -1
			if len(pe.Trajectory) > 0 {
				projEndFrame = frameToV1(pe.Trajectory[len(pe.Trajectory)-1].FrameNum)
			}

			markers = append(markers, []any{
				markerType,
				text,
				frameToV1(pe.CaptureFrame),
				projEndFrame,
				int(pe.FirerObjectID),
				color,
				-1,
				posArray,
				[]float64{1, 1},
				"ICON",
				"Solid",
			})
		}

		if len(pe.Hits) > 0 {
			weaponName := pe.MuzzleDisplay
			if weaponName == "" {
				weaponName = pe.WeaponDisplay
			}
			eventText := util.FormatWeaponText("", weaponName, pe.MagazineDisplay)

			var startPos core.Position3D
			if len(pe.Trajectory) > 0 {
				startPos = pe.Trajectory[0].Position
			}

			for _, hit := range pe.Hits {
				var victimID uint
				if hit.SoldierID != nil {
					victimID = uint(*hit.SoldierID)
				} else if hit.VehicleID != nil {
					victimID = uint(*hit.VehicleID)
				}

				dx := startPos.X - hit.Position.X
				dy := startPos.Y - hit.Position.Y
				dist := float32(math.Sqrt(dx*dx + dy*dy))

				events = append(events, []any{
					frameToV1(hit.CaptureFrame),
					"hit",
					victimID,
					[]any{uint(pe.FirerObjectID), eventText},
					dist,
				})
			}
		}
	}

	return events, markers, firelines
}
