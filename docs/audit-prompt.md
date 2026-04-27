# Dashboard Implementation Audit Prompt

**Purpose:** This prompt is designed for an independent agent to audit the backend-frontend integration work completed for the Jellywatch dashboard.

**Context:** A previous session implemented the dashboard API endpoints, authentication, media manager integration, and static file serving. This audit should verify correctness, security, and code quality.

---

## Audit Instructions

You are auditing a Go + React project called Jellywatch - a media library organization tool with a web dashboard. The previous implementation session completed backend-frontend integration tasks. Your job is to thoroughly inspect the implementation for correctness, security, and best practices.

### Project Context

- **Language:** Go 1.21+ backend, React 18 frontend
- **Framework:** Chi router for HTTP, TanStack Query for data fetching
- **Database:** SQLite with custom ORM-like layer
- **External Services:** Sonarr, Radarr (media managers)

### Files to Audit

```
internal/api/
├── server.go           # Server setup, routing, middleware
├── handlers.go         # All API endpoint handlers
├── auth.go             # Authentication logic
├── media_managers.go   # Sonarr/Radarr client wrappers
└── dashboard.go        # Dashboard endpoint

internal/config/config.go   # Password field added
internal/activity/logger.go # GetRecentEntries method added
api/openapi.yaml            # API specification
api/types.gen.go            # Generated types
api/server.gen.go           # Generated server interface
```

### Audit Checklist

#### 1. API Correctness

- [ ] All endpoints defined in `api/openapi.yaml` are implemented
- [ ] Handler function signatures match `api.ServerInterface`
- [ ] Response types match generated `api` types
- [ ] HTTP status codes are appropriate (200, 201, 400, 401, 404, 500, 502)
- [ ] Query parameters are correctly parsed
- [ ] Request body parsing handles malformed JSON gracefully

#### 2. Security Review

- [ ] **Authentication:**
  - Password comparison uses constant-time comparison (check `auth.go`)
  - Session tokens are cryptographically random (32 bytes minimum)
  - Session cookies have `HttpOnly` flag
  - Session cookies have `Secure` flag (or note it's configurable)
  - Session expiration is reasonable (24 hours max)
  - Expired sessions are cleaned up

- [ ] **Authorization:**
  - Auth middleware correctly identifies public vs protected paths
  - Protected endpoints return 401 when auth required but missing
  - Auth disabled mode (no password) works correctly

- [ ] **Input Validation:**
  - File IDs are validated before database operations
  - Manager IDs are checked against config before API calls
  - User input is not directly interpolated into SQL queries

- [ ] **CORS:**
  - CORS configuration is appropriate
  - Credentials mode is handled correctly

- [ ] **Error Messages:**
  - Error messages don't leak sensitive information
  - Stack traces are not exposed to clients

#### 3. Code Quality

- [ ] **Error Handling:**
  - All errors are handled (no ignored error returns)
  - Errors are logged where appropriate
  - User-facing errors are helpful but not verbose

- [ ] **Concurrency:**
  - Session store uses mutex correctly
  - Scan state uses mutex correctly
  - No race conditions in background goroutines

- [ ] **Resource Management:**
  - HTTP response bodies are closed
  - Database connections are properly managed
  - File handles are closed
  - Goroutines don't leak

- [ ] **Code Organization:**
  - Handlers are focused on HTTP concerns
  - Business logic is in service layer
  - Database operations are in database layer

#### 4. Integration Testing

- [ ] Build the project: `go build ./cmd/jellywatch`
- [ ] Run tests: `go test ./internal/api/...`
- [ ] Start server: `./jellywatch serve --addr :8765`
- [ ] Test each endpoint:

```bash
# Health check
curl http://localhost:8765/api/v1/health

# Auth status
curl http://localhost:8765/api/v1/auth/status

# Dashboard
curl http://localhost:8765/api/v1/dashboard

# Duplicates
curl http://localhost:8765/api/v1/duplicates

# Scattered
curl http://localhost:8765/api/v1/scattered

# Activity
curl http://localhost:8765/api/v1/activity

# Scan status
curl http://localhost:8765/api/v1/scan/status

# Media managers
curl http://localhost:8765/api/v1/media-managers

# Static files (SPA)
curl http://localhost:8765/
```

#### 5. API Specification Compliance

- [ ] Read `api/openapi.yaml` to understand the API contract
- [ ] Verify each endpoint matches the specification
- [ ] Check response schemas match types in `api/types.gen.go`
- [ ] Verify required fields are always returned
- [ ] Verify optional fields are handled correctly (omitempty)

#### 6. Specific Areas to Review

**A. Authentication (`internal/api/auth.go`)**

```go
// Review these specific areas:

// 1. Token generation - is it secure?
func generateToken() string {
    bytes := make([]byte, SessionTokenLength)
    if _, err := rand.Read(bytes); err != nil {
        // Fallback - is this secure?
        return hex.EncodeToString([]byte(time.Now().String()))
    }
    return hex.EncodeToString(bytes)
}

// 2. Password comparison - timing attack safe?
if req.Password != s.cfg.Password {
    // Should use subtle.ConstantTimeCompare
}

// 3. Session store - thread safe?
func (s *SessionStore) Get(token string) (*Session, bool) {
    s.mu.RLock()
    session, exists := s.sessions[token]
    s.mu.RUnlock()
    // Check expiration, then delete if expired
    // Is there a race condition here?
}
```

**B. Media Manager Integration (`internal/api/media_managers.go`)**

```go
// Review:

// 1. Error handling for external API calls
func (s *Server) GetMediaManagerQueue(...) {
    items, err := client.GetAllQueueItems()
    if err != nil {
        // Is 502 the right status code?
        writeError(w, http.StatusBadGateway, ...)
    }
}

// 2. Manager ID validation
func isManagerConfigured(cfg *config.Config, id string) bool {
    // Is "sonarr" vs "SONARR" handled?
    // What about empty string?
}
```

**C. Scan Implementation (`internal/api/handlers.go`)**

```go
// Review:

// 1. Concurrent scan prevention
func (s *Server) StartScan(...) {
    scanState.mu.Lock()
    defer scanState.mu.Unlock()

    if scanState.status == "scanning" {
        // Returns 409 Conflict - is this appropriate?
    }

    scanState.status = "scanning"
    go s.runBackgroundScan()  // Does this race with the unlock?
}

// 2. Background goroutine cleanup
func (s *Server) runBackgroundScan() {
    defer func() {
        scanState.mu.Lock()
        scanState.status = "idle"
        scanState.mu.Unlock()
    }()
    // What if there's a panic?
}
```

**D. Static File Serving (`internal/api/server.go`)**

```go
// Review:

// 1. Path traversal protection
func spaFileServer(fs embed.FS) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        path := r.URL.Path
        // Is there path traversal vulnerability?
        // What about "../../../../etc/passwd"?
    }
}

// 2. MIME type handling
// Are correct Content-Type headers set?
```

### Audit Report Format

After completing the audit, produce a report with:

1. **Summary** - Overall assessment (PASS/FAIL with conditions)

2. **Critical Issues** - Security vulnerabilities that must be fixed immediately

3. **High Priority Issues** - Bugs or design flaws that could cause problems

4. **Medium Priority Issues** - Code quality improvements

5. **Low Priority Issues** - Nice-to-have improvements

6. **Verified Correct** - What was checked and found to be correct

7. **Recommendations** - Suggested improvements

### Expected Findings

Based on the implementation, specifically check for:

1. **Password comparison** - Uses `!=` instead of `subtle.ConstantTimeCompare`

2. **Token fallback** - Falls back to timestamp-based token if `rand.Read` fails

3. **Session store race** - Potential race between checking expiration and deleting

4. **Scan goroutine** - No panic recovery in background goroutine

5. **Static file paths** - Embedded FS should prevent path traversal, but verify

6. **Error responses** - Check if error responses match API spec format

### How to Start

1. Read `docs/dashboard-implementation.md` for context
2. Read `api/openapi.yaml` to understand the API contract
3. Read each file in `internal/api/` in order:
   - `server.go` (setup)
   - `auth.go` (authentication)
   - `handlers.go` (main endpoints)
   - `media_managers.go` (external services)
   - `dashboard.go` (dashboard specific)
4. Build and test the server
5. Produce audit report

---

## Summary for Auditor

You are auditing a Go backend that:
- Serves a React SPA with embedded static files
- Provides REST API endpoints for media library management
- Integrates with Sonarr/Radarr for queue management
- Uses simple password-based session authentication
- Logs activity to JSONL files

Focus on:
1. Security (auth, input validation, CORS)
2. Concurrency (mutex usage, goroutine safety)
3. Error handling (completeness, user-friendliness)
4. API compliance (matches OpenAPI spec)

Report all findings with specific file locations and line numbers.
