package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/OCAP2/extension/v5/internal/api"
	"github.com/OCAP2/extension/v5/internal/storage"
	"github.com/OCAP2/extension/v5/pkg/a3interface"
)

// missionSaveState holds the cross-call state for the async save worker.
// There is exactly one instance, owned by the package-level saveState variable.
type missionSaveState struct {
	inFlight atomic.Bool
}

// saveState is the package-level state used by the :MISSION:SAVE: handler.
var saveState = &missionSaveState{}

// triggerMissionSave is the logic behind the :MISSION:SAVE: dispatcher handler,
// extracted so it can be exercised in tests with a fake backend and a fake
// api client.
//
// tag is the optional save-time tag override from the addon; it is forwarded
// to EndMission so streaming and export backends can apply it.
//
// Contract:
//   - Returns "queued" + nil error when a goroutine was spawned.
//   - Returns a non-nil error if another save is already in progress.
//   - Never blocks on the save itself. The save runs asynchronously and
//     reports completion via a3interface.WriteArmaCallback with function
//     name ":MISSION:SAVED:".
func triggerMissionSave(state *missionSaveState, backend storage.Backend, client *api.Client, tag string) (any, error) {
	if backend == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	if !state.inFlight.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("save already in progress")
	}

	go runMissionSaveWorker(state, backend, client, tag)
	return "queued", nil
}

// triggerMissionDiscard is the logic behind the :MISSION:DISCARD: dispatcher
// handler: the addon ended a recording that does not meet its save limits.
// It shares the in-flight guard with saves, as both end the mission.
//
// Contract:
//   - Returns "queued" + nil error when a goroutine was spawned; it reports
//     completion via a ":MISSION:DISCARDED:" callback: ["ok"], ["kept"] for
//     a backend that keeps what it recorded, or ["error", message].
//   - Returns a non-nil error if a save or discard is already in progress.
func triggerMissionDiscard(state *missionSaveState, backend storage.Backend) (any, error) {
	if backend == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	if !state.inFlight.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("save already in progress")
	}

	go runMissionDiscardWorker(state, backend)
	return "queued", nil
}

// runMissionDiscardWorker performs DiscardMission, recovers from panics and
// dispatches the :MISSION:DISCARDED: callback.
func runMissionDiscardWorker(state *missionSaveState, backend storage.Backend) {
	defer state.inFlight.Store(false)

	d, ok := backend.(storage.Discardable)
	if !ok {
		if Logger != nil {
			Logger.Info("Storage backend keeps discarded missions")
		}
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:DISCARDED:", "kept")
		return
	}

	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic during mission discard: %v", r)
				if Logger != nil {
					Logger.Error("panic during mission discard", "panic", r, "stack", string(debug.Stack()))
				}
			}
		}()
		return d.DiscardMission()
	}()
	if err != nil {
		if Logger != nil {
			Logger.Error("Failed to discard mission in storage backend", "error", err)
		}
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:DISCARDED:", "error", err.Error())
		return
	}
	if Logger != nil {
		Logger.Info("Mission recording discarded")
	}
	_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:DISCARDED:", "ok")
}

// runMissionSaveWorker performs EndMission + optional upload, recovers from
// panics, flushes OTel, and dispatches the :MISSION:SAVED: callback.
func runMissionSaveWorker(state *missionSaveState, backend storage.Backend, client *api.Client, tag string) {
	// Always clear the in-flight flag before returning.
	defer state.inFlight.Store(false)

	start := time.Now()

	var (
		savedPath string
		endErr    error
		uploadErr error
		panicInfo string
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicInfo = fmt.Sprintf("panic during mission save: %v", r)
				if Logger != nil {
					Logger.Error("panic during mission save",
						"panic", r,
						"stack", string(debug.Stack()),
					)
				}
			}
		}()

		if err := backend.EndMission(tag); err != nil {
			endErr = err
			if Logger != nil {
				Logger.Error("Failed to end mission in storage backend", "error", err)
			}
			return
		}
		if Logger != nil {
			Logger.Info("Mission recording saved to storage backend",
				"duration", time.Since(start),
			)
		}

		if u, ok := backend.(storage.Uploadable); ok && client != nil {
			if path := u.GetExportedFilePath(); path != "" {
				savedPath = path
				meta := u.GetExportMetadata()
				if err := client.Upload(path, meta); err != nil {
					uploadErr = err
					if Logger != nil {
						Logger.Error("Failed to upload to OCAP web",
							"error", err,
							"path", path,
						)
					}
				} else if Logger != nil {
					Logger.Info("Mission uploaded to OCAP web",
						"path", path,
						"duration", time.Since(start),
					)
				}
			}
		} else if u, ok := backend.(storage.Uploadable); ok {
			// Client not configured — still report the path so the addon can inform players.
			savedPath = u.GetExportedFilePath()
		}
	}()

	// Flush OTel data if provider is available.
	if OTelProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := OTelProvider.Flush(ctx); err != nil && Logger != nil {
			Logger.Warn("Failed to flush OTel data", "error", err)
		}
	}

	// Build callback payload.
	switch {
	case panicInfo != "":
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:SAVED:", "error", panicInfo)
	case endErr != nil:
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:SAVED:", "error", endErr.Error())
	case uploadErr != nil:
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:SAVED:", "partial", savedPath, uploadErr.Error())
	default:
		_ = a3interface.WriteArmaCallback(ExtensionName, ":MISSION:SAVED:", "ok", savedPath)
	}
}
