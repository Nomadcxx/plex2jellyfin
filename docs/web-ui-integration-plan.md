# Web UI Integration Plan

## Current State Analysis

### What Exists ✅
- **API Server** (`internal/api/server.go`): Full REST API with routing
- **Authentication**: Session-based auth, 24h TTL, HTTP-only cookies
- **Next.js Web UI** (`web/`): Built and embedded via `go:embed`
- **Daemon** (`cmd/plex2jellyfin-daemon/`): Runs file watcher & periodic scanner
- **Systemd Service**: Unit file at `systemd/plex2jellyfin-daemon.service`

### What's Missing ❌
- API server not started in daemon
- Web UI port (:8080) not exposed
- Installer doesn't show web UI access info
- No default credentials setup

## Implementation Tasks

### Phase 1: Start API Server in Daemon

**File:** `cmd/plex2jellyfin-daemon/main.go`

Add API server goroutine alongside existing watcher:

```go
import "plex2jellyfin/internal/api"

// In main() or run():
apiServer := api.NewServer(cfg, db)
go func() {
    if err := apiServer.Start(":8080"); err != nil {
        log.Printf("API server error: %v", err)
    }
}()
```

### Phase 2: Update Systemd Service

**File:** `systemd/plex2jellyfin-daemon.service`

Ensure port 8080 is documented:

```ini
[Service]
# ...existing config...
# Web UI available at http://localhost:8080
```

### Phase 3: Installer Completion Screen

**File:** `cmd/installer/screens.go` - `renderComplete()`

Add web UI section after "What's Next?":

```
╭──────────────────────────────────────────────────────╮
│  🌐 Web Interface                                    │
├──────────────────────────────────────────────────────┤
│  URL:      http://localhost:8080                     │
│  Status:   systemctl status plex2jellyfin-daemon              │
│                                                      │
│  Auth:     [If password configured]                  │
│            Username: admin                           │
│            Password: (set in config.toml)            │
╰──────────────────────────────────────────────────────╯
```

### Phase 4: Config Updates

**File:** `internal/config/config.go`

Ensure WebUI config section exists:

```go
type WebConfig struct {
    Port     int    `toml:"port"`      // default 8080
    Password string `toml:"password"`  // empty = no auth
}
```

## Auth Behavior

| Password in Config | Behavior |
|--------------------|----------|
| Empty/not set | No authentication required |
| Set | Login required, session-based |

## Network Access

For LAN access, installer should detect local IP and show:
- `http://localhost:8080` (local)
- `http://192.168.x.x:8080` (LAN)

## Verification Steps

1. `make all` - builds everything
2. `sudo systemctl restart plex2jellyfin-daemon`
3. `curl http://localhost:8080/api/health` - should return OK
4. Open browser to http://localhost:8080 - should show UI

## Security Considerations

- Web UI runs as root (same as daemon) for file operations
- Consider binding to localhost only by default
- Add `bind_address` config option for LAN exposure
