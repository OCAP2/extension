// Package v1 contains the v1 export format for OCAP2 mission data.
// This format is compatible with the ocap2-web frontend.
package v1

// Export is the root JSON structure for v1 format
// Note: Markers uses capital M for compatibility with ocap2-web
type Export struct {
	AddonVersion     string     `json:"addonVersion"`
	ExtensionVersion string     `json:"extensionVersion"`
	ExtensionBuild   string     `json:"extensionBuild"`
	MissionName      string     `json:"missionName"`
	MissionAuthor    string     `json:"missionAuthor"`
	WorldName        string     `json:"worldName"`
	EndFrame         int        `json:"endFrame"`
	CaptureDelay     float32    `json:"captureDelay"`
	Tags             string     `json:"tags"`
	Times            []Time     `json:"times"`
	Entities         []Entity   `json:"entities"`
	Events           [][]any    `json:"events"`
	Markers          [][]any    `json:"Markers"` // Capital M for ocap2-web compatibility
}

// Time represents time synchronization data for a frame
type Time struct {
	Date           string  `json:"date"`
	FrameNum       int     `json:"frameNum"`
	SystemTimeUTC  string  `json:"systemTimeUTC"`
	Time           float32 `json:"time"`
	TimeMultiplier float32 `json:"timeMultiplier"`
}

// Entity represents a soldier or vehicle
type Entity struct {
	ID            uint16  `json:"id"`
	Name          string  `json:"name"`
	Group         string  `json:"group,omitempty"`
	Side          string  `json:"side"`
	IsPlayer      int     `json:"isPlayer"`
	Type          string  `json:"type"`
	Role          string  `json:"role,omitempty"`
	Class         string  `json:"class,omitempty"`
	StartFrameNum int     `json:"startFrameNum"`
	Positions     [][]any `json:"positions"`
	FramesFired   [][]any `json:"framesFired"`
}
