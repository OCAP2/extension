package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OCAP2/extension/v5/internal/model"
)

func main() {
	var err error
	Logger.Info("Starting up...")

	Logger.Info("Initializing storage...")
	err = initStorage()
	if err != nil {
		panic(err)
	}
	Logger.Info("Storage initialization complete.")
	initExtension()

	args := os.Args[1:]
	if len(args) > 0 {
		if strings.ToLower(args[0]) == "getjson" {
			missionIds := args[1:]
			if len(missionIds) > 0 {
				err = getOcapRecording(missionIds)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Println("No mission IDs provided.")
			}
		}
		if strings.ToLower(args[0]) == "reducemission" {
			missionIds := args[1:]
			if len(missionIds) > 0 {
				err = reduceMission(missionIds)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Println("No mission IDs provided.")
			}
		}
		if strings.ToLower(args[0]) == "testquery" {
			err = testQuery()
			if err != nil {
				panic(err)
			}
		}
	} else {
		fmt.Println("No arguments provided.")
	}
	fmt.Scanln()
}

func getOcapRecording(missionIDs []string) (err error) {
	fmt.Println("Getting JSON for mission IDs: ", missionIDs)

	for _, missionID := range missionIDs {
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		txStart := time.Now()
		var mission model.Mission
		ocapMission := make(map[string]any)
		err = DB.Model(&model.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return err
		}

		ocapMission["addonVersion"] = mission.AddonVersion
		ocapMission["extensionVersion"] = mission.ExtensionVersion
		ocapMission["extensionBuild"] = mission.ExtensionBuild
		ocapMission["ocapRecorderExtensionVersion"] = mission.OcapRecorderExtensionVersion

		ocapMission["missionAuthor"] = mission.Author
		ocapMission["missionName"] = mission.OnLoadName
		if mission.OnLoadName == "" {
			ocapMission["missionName"] = mission.MissionName
		}

		world := model.World{}
		err = DB.Model(&model.World{}).Where("id = ?", mission.WorldID).First(&world).Error
		if err != nil {
			return fmt.Errorf("error getting world: %w", err)
		}
		ocapMission["worldName"] = world.WorldName

		totalSoldiers := int64(0)
		err = DB.Model(&model.Soldier{}).Where("mission_id = ?", missionIDInt).Count(&totalSoldiers).Error
		if err != nil {
			return fmt.Errorf("error getting soldier count: %w", err)
		}

		totalVehicles := int64(0)
		err = DB.Model(&model.Vehicle{}).Where("mission_id = ?", missionIDInt).Count(&totalVehicles).Error
		if err != nil {
			return fmt.Errorf("error getting vehicle count: %w", err)
		}

		ocapMission["Rone"] = map[string]any{}
		ocapMission["events"] = []any{}

		soldiers := []model.Soldier{}
		soldierTxStart := time.Now()
		err = DB.Model(&model.Soldier{}).Where("mission_id = ?", missionIDInt).Find(&soldiers).Error
		if err != nil {
			return fmt.Errorf("error getting soldiers: %w", err)
		}
		fmt.Println("Got soldiers in ", time.Since(soldierTxStart))

		entities := []map[string]any{}
		for _, soldier := range soldiers {
			entity := map[string]any{}
			entity["id"] = soldier.ObjectID
			entity["name"] = soldier.UnitName
			entity["group"] = soldier.GroupID
			entity["side"] = soldier.Side
			entity["isPlayer"] = 0
			if soldier.IsPlayer {
				entity["isPlayer"] = 1
			}
			entity["type"] = "unit"
			entity["startFrameNum"] = soldier.JoinFrame

			soldierStates := []model.SoldierState{}
			err = DB.Model(&model.SoldierState{}).
				Where("mission_id = ? AND soldier_ocap_id = ?", missionIDInt, soldier.ObjectID).
				Order("capture_frame ASC").
				Find(&soldierStates).Error
			if err != nil {
				return fmt.Errorf("error getting soldier states: %w", err)
			}

			positions := []any{}
			for _, state := range soldierStates {
				coord, _ := state.Position.Coordinates()
				position := []any{
					[]float64{coord.XY.X, coord.XY.Y},
					state.Bearing,
					state.Lifestate,
					state.InVehicleObjectID,
					state.UnitName,
					state.IsPlayer,
					state.CurrentRole,
				}
				positions = append(positions, position)
			}
			entity["positions"] = positions

			projectileEvents := []model.ProjectileEvent{}
			err = DB.Model(&model.ProjectileEvent{}).
				Where("mission_id = ? AND firer_object_id = ?", missionIDInt, soldier.ObjectID).
				Order("capture_frame ASC").
				Find(&projectileEvents).Error
			if err != nil {
				return fmt.Errorf("error getting projectile events: %w", err)
			}

			framesFired := []any{}
			for _, event := range projectileEvents {
				var startX, startY, endX, endY float64
				if !event.Positions.IsEmpty() {
					if ls, ok := event.Positions.AsLineString(); ok {
						seq := ls.Coordinates()
						if seq.Length() > 0 {
							start := seq.Get(0)
							startX, startY = start.X, start.Y
							end := seq.Get(seq.Length() - 1)
							endX, endY = end.X, end.Y
						}
					}
				}
				frameFired := []any{
					event.CaptureFrame,
					[]float64{endX, endY},
					[]float64{startX, startY},
					event.WeaponDisplay,
					event.MagazineDisplay,
					event.Mode,
				}
				framesFired = append(framesFired, frameFired)
			}
			entity["framesFired"] = framesFired

			entities = append(entities, entity)
		}

		vehicles := []model.Vehicle{}
		err = DB.Model(&model.Vehicle{}).Where("mission_id = ?", missionIDInt).Find(&vehicles).Error
		if err != nil {
			return fmt.Errorf("error getting vehicles: %w", err)
		}
		for _, vehicle := range vehicles {
			entity := map[string]any{}
			entity["id"] = vehicle.ObjectID
			entity["name"] = vehicle.DisplayName
			entity["class"] = vehicle.ClassName
			entity["side"] = "UNKNOWN"
			entity["type"] = vehicle.OcapType
			entity["startFrameNum"] = vehicle.JoinFrame

			vehicleStates := []model.VehicleState{}
			err = DB.Model(&model.VehicleState{}).
				Where("mission_id = ? AND vehicle_ocap_id = ?", missionIDInt, vehicle.ObjectID).
				Order("capture_frame ASC").
				Find(&vehicleStates).Error
			if err != nil {
				return fmt.Errorf("error getting vehicle states: %w", err)
			}
			positions := []any{}
			for _, state := range vehicleStates {
				coord, _ := state.Position.Coordinates()
				var crew any
				if err := json.Unmarshal([]byte(state.Crew), &crew); err != nil {
					crew = []any{}
				}
				position := []any{
					[]float64{coord.XY.X, coord.XY.Y},
					state.Bearing,
					state.IsAlive,
					crew,
				}
				positions = append(positions, position)
			}
			entity["positions"] = positions
			entity["framesFired"] = []any{}

			entities = append(entities, entity)
		}

		ocapMission["entities"] = entities

		// Compute endFrame from the maximum capture_frame across all states
		var endFrame uint
		DB.Model(&model.SoldierState{}).Where("mission_id = ?", missionIDInt).Select("COALESCE(MAX(capture_frame), 0)").Scan(&endFrame)
		ocapMission["endFrame"] = endFrame

		fmt.Println("Got mission data in ", time.Since(txStart))

		ocapMissionJSON, err := json.Marshal(ocapMission)
		if err != nil {
			return fmt.Errorf("error marshalling mission data: %w", err)
		}

		fileName := fmt.Sprintf("%s_%s.json.gz", mission.MissionName, mission.StartTime.Format("20060102_150405"))
		fileName = strings.ReplaceAll(fileName, " ", "_")
		fileName = strings.ReplaceAll(fileName, ":", "_")
		f, err := os.Create(fileName)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer f.Close()

		gzWriter := gzip.NewWriter(f)
		defer gzWriter.Close()
		_, err = gzWriter.Write(ocapMissionJSON)
		if err != nil {
			return fmt.Errorf("error writing to gzip: %w", err)
		}

		fmt.Println("Wrote mission data to ", fileName)
	}

	return nil
}

func reduceMission(missionIDs []string) (err error) {
	for _, missionID := range missionIDs {
		missionIDInt, err := strconv.Atoi(missionID)
		if err != nil {
			return err
		}

		txStart := time.Now()
		var mission model.Mission
		err = DB.Model(&model.Mission{}).Where("id = ?", missionIDInt).First(&mission).Error
		if err != nil {
			return fmt.Errorf("error getting mission: %w", err)
		}

		soldierStatesToDelete := []model.SoldierState{}
		err = DB.Model(&model.SoldierState{}).Where(
			"mission_id = ? AND capture_frame % 5 != 0",
			mission.ID,
		).Order("capture_frame ASC").Find(&soldierStatesToDelete).Error
		if err != nil {
			return fmt.Errorf("error getting soldier states to delete: %w", err)
		}

		if len(soldierStatesToDelete) == 0 {
			fmt.Println("No soldier states to delete for missionId ", missionID, ", checked in ", time.Since(txStart))
			continue
		}

		err = DB.Delete(&soldierStatesToDelete).Error
		if err != nil {
			return fmt.Errorf("error deleting soldier states: %w", err)
		}

		fmt.Println("Deleted ", len(soldierStatesToDelete), " soldier states from missionId ", missionID, " in ", time.Since(txStart))
	}

	fmt.Println("")
	fmt.Println("----------------------------------------")
	fmt.Println("")
	fmt.Println("Finished reducing soldier states, running VACUUM to recover space...")
	txStart := time.Now()
	tables := []string{}
	err = DB.Raw(
		`SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`,
	).Scan(&tables).Error
	if err != nil {
		return fmt.Errorf("error getting tables to vacuum: %w", err)
	}

	for _, table := range tables {
		err = DB.Raw(fmt.Sprintf("VACUUM (FULL) %s", table)).Error
		if err != nil {
			return fmt.Errorf("error running VACUUM on table %s: %w", table, err)
		}
	}

	fmt.Println("Finished VACUUM in ", time.Since(txStart))
	fmt.Println("Finished reducing, press enter to exit.")

	return nil
}

func testQuery() (err error) {
	query := `
select
    s.ocap_id,
    ss.capture_frame,
    json_agg(ss.*) as states,
    json_agg(hit.*) as hits,
    json_agg(kill.*) as kills,
    json_agg(fire.*) as fired,
    json_agg(re.*) as radio,
    json_agg(ce.*) as chat
from soldiers s
  left join (
    select *
    from soldier_states
    order by capture_frame asc
  ) ss on ss.soldier_id = s.id
  left join kill_events kill on (
    kill.victim_id_soldier = s.id
    or kill.killer_id_soldier = s.id
  )
  and ss.capture_frame = kill.capture_frame
  left join hit_events hit on (
    hit.victim_id_soldier = s.id
    or hit.shooter_id_soldier = s.id
  )
  and ss.capture_frame = hit.capture_frame
  left join fired_events fire on fire.soldier_id = s.id
  and ss.capture_frame = fire.capture_frame
  left join radio_events re on re.soldier_id = s.id
  and ss.capture_frame = re.capture_frame
  left join chat_events ce on ce.soldier_id = s.id
  and ss.capture_frame = ce.capture_frame
where s.mission_id = ? and ss.capture_frame between ? and ?
group by s.ocap_id,
  ss.capture_frame
order by s.ocap_id,
  ss.capture_frame;
`

	frameData := []model.FrameData{}
	err = DB.Raw(query, 4, 0, 100).Scan(&frameData).Error
	if err != nil {
		fmt.Println(err)
		return
	}

	jsonBytes, err := json.Marshal(frameData)
	if err != nil {
		return err
	}
	err = os.WriteFile("test.json", jsonBytes, 0644)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	return nil
}
