# API Upload Design

Upload mission JSON to OCAP web frontend after export.

## Decisions

- **Local-first**: Write to disk first, then upload. File remains if upload fails.
- **No retry**: Log failures, admin handles manually. Keep it simple.
- **New package**: `internal/api/` for API client (healthcheck + upload).
- **Caller handles upload**: Storage backend returns path, handler triggers upload.
- **Optional interface**: `Uploadable` interface for backends that produce uploadable files.

## Package Structure

```
internal/api/
├── client.go      # API client with Healthcheck() and Upload()
└── client_test.go
```

## Interfaces

### storage.Uploadable

Optional interface in `internal/storage/storage.go`:

```go
type Uploadable interface {
    GetExportedFilePath() string
    GetExportMetadata() UploadMetadata
}

type UploadMetadata struct {
    WorldName       string
    MissionName     string
    MissionDuration float64
    Tag             string
}
```

Memory backend implements this. Database backend does not.

## API Client

`internal/api/client.go`:

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func New(baseURL, apiKey string) *Client

func (c *Client) Healthcheck() error

func (c *Client) Upload(filePath string, metadata UploadMetadata) error
```

- 30 second timeout for uploads
- `apiKey` maps to `secret` field in upload form
- Upload uses multipart/form-data (POST /api/v1/operations/add)

## Upload Form Fields

Per ocap2-web API:

| Field | Source |
|-------|--------|
| `secret` | config `api.apiKey` |
| `filename` | base name of exported file |
| `worldName` | `UploadMetadata.WorldName` |
| `missionName` | `UploadMetadata.MissionName` |
| `missionDuration` | `UploadMetadata.MissionDuration` |
| `tag` | `UploadMetadata.Tag` |
| `file` | gzipped JSON file |

## Integration

### Initialization (main.go)

```go
var apiClient *api.Client

func initAPIClient() {
    serverURL := viper.GetString("api.serverUrl")
    if serverURL == "" {
        Logger.Info("API server URL not configured, upload disabled")
        return
    }

    apiKey := viper.GetString("api.apiKey")
    apiClient = api.New(serverURL, apiKey)

    if err := apiClient.Healthcheck(); err != nil {
        Logger.Info("OCAP Frontend is offline", "error", err)
    } else {
        Logger.Info("OCAP Frontend is online")
    }
}
```

### :SAVE: Handler

```go
d.Register(":SAVE:", func(e dispatcher.Event) (any, error) {
    if storageBackend != nil {
        if err := storageBackend.EndMission(); err != nil {
            return nil, err
        }

        if u, ok := storageBackend.(storage.Uploadable); ok && apiClient != nil {
            if path := u.GetExportedFilePath(); path != "" {
                meta := u.GetExportMetadata()
                if err := apiClient.Upload(path, meta); err != nil {
                    Logger.Error("Failed to upload", "error", err, "path", path)
                } else {
                    Logger.Info("Mission uploaded", "path", path)
                }
            }
        }
    }
    return "ok", nil
})
```

Upload failure is logged but does not fail the command.

## Memory Backend Changes

`internal/storage/memory/memory.go`:

- Add `lastExportPath string` field
- Set in `exportJSON()` after successful write
- Reset in `StartMission()`
- Implement `GetExportedFilePath()` and `GetExportMetadata()`

Duration calculation:
```go
float64(s.mission.EndFrame) * float64(s.mission.CaptureDelay) / 1000.0
```

## File Changes

**Create:**
- `internal/api/client.go`

**Modify:**
- `internal/storage/storage.go` - Add Uploadable, UploadMetadata
- `internal/storage/memory/memory.go` - Implement Uploadable
- `cmd/ocap_recorder/main.go` - Init API client, update :SAVE: handler, remove checkServerStatus()

**No changes:**
- Database storage backend
- Configuration structure
