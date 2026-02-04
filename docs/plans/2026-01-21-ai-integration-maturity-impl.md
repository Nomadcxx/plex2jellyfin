# AI Integration Maturity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Mature AI integration from prototype to production with reliability, observability, and extended use cases.

**Architecture:** Circuit breaker protects pipeline from AI failures. Background queue processes enhancements asynchronously. Response recovery chain handles malformed outputs. All AI flows through single Integrator interface.

**Tech Stack:** Go 1.21+, SQLite, Ollama API, bubbletea (for CLI)

**Reference:** Design document at `docs/plans/2026-01-21-ai-integration-maturity.md`

---

## Phase 1: Code Cleanup

### Task 1: Remove Dead Code (ai_naming.go)

**Files:**
- Delete: `internal/naming/ai_naming.go`
- Modify: Any files importing it (verify none do)

**Step 1: Verify no imports**

```bash
grep -r "ParseMovieNameWithAI\|ParseTVShowNameWithAI\|NewAIMatcher" --include="*.go" internal/ cmd/
```

Expected: No matches (only the file itself)

**Step 2: Delete the dead code file**

```bash
rm internal/naming/ai_naming.go
```

**Step 3: Verify tests still pass**

```bash
go test ./internal/naming/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add -A && git commit -m "refactor(ai): remove dead ai_naming.go code"
```

---

### Task 2: Remove Duplicate AI Config (internal/ai/config.go)

The `internal/ai/config.go` duplicates `internal/config/config.go` AIConfig. Remove it.

**Files:**
- Delete: `internal/ai/config.go`
- Modify: `internal/ai/integrator.go` - use `config.AIConfig`
- Modify: `internal/ai/matcher.go` - use `config.AIConfig`

**Step 1: Check what Config fields are used in ai package**

```bash
grep -n "cfg\.\|config\.\|Config\." internal/ai/*.go | grep -v "_test.go"
```

Document which fields are needed.

**Step 2: Update integrator.go imports and types**

Change:
```go
import (
    // ... existing imports
)
```

To:
```go
import (
    "github.com/Nomadcxx/jellywatch/internal/config"
    // ... existing imports
)
```

Replace `Config` with `config.AIConfig` throughout.

**Step 3: Update matcher.go imports and types**

Same pattern - replace local Config with config.AIConfig.

**Step 4: Delete internal/ai/config.go**

```bash
rm internal/ai/config.go
```

**Step 5: Verify build**

```bash
go build ./...
```

Expected: Success

**Step 6: Run tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add -A && git commit -m "refactor(ai): consolidate config into internal/config"
```

---

### Task 3: Add Circuit Breaker Config to AIConfig

**Files:**
- Modify: `internal/config/config.go:120-130`

**Step 1: Write test for new config fields**

Create/modify `internal/config/config_test.go`:

```go
func TestAIConfig_CircuitBreakerDefaults(t *testing.T) {
    cfg := DefaultAIConfig()

    if cfg.CircuitBreaker.FailureThreshold != 5 {
        t.Errorf("expected failure threshold 5, got %d", cfg.CircuitBreaker.FailureThreshold)
    }
    if cfg.CircuitBreaker.FailureWindowSeconds != 120 {
        t.Errorf("expected failure window 120s, got %d", cfg.CircuitBreaker.FailureWindowSeconds)
    }
    if cfg.CircuitBreaker.CooldownSeconds != 30 {
        t.Errorf("expected cooldown 30s, got %d", cfg.CircuitBreaker.CooldownSeconds)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -run TestAIConfig_CircuitBreakerDefaults -v
```

Expected: FAIL (CircuitBreaker field doesn't exist)

**Step 3: Add CircuitBreakerConfig struct and fields**

In `internal/config/config.go`, add after AIConfig:

```go
type CircuitBreakerConfig struct {
    FailureThreshold     int `mapstructure:"failure_threshold"`
    FailureWindowSeconds int `mapstructure:"failure_window_seconds"`
    CooldownSeconds      int `mapstructure:"cooldown_seconds"`
}

type KeepaliveConfig struct {
    Enabled           bool `mapstructure:"enabled"`
    IntervalSeconds   int  `mapstructure:"interval_seconds"`
    IdleTimeoutSeconds int `mapstructure:"idle_timeout_seconds"`
}
```

Update AIConfig:

```go
type AIConfig struct {
    Enabled             bool                 `mapstructure:"enabled"`
    OllamaEndpoint      string               `mapstructure:"ollama_endpoint"`
    Model               string               `mapstructure:"model"`
    ConfidenceThreshold float64              `mapstructure:"confidence_threshold"`
    TimeoutSeconds      int                  `mapstructure:"timeout_seconds"`
    CacheEnabled        bool                 `mapstructure:"cache_enabled"`
    CloudModel          string               `mapstructure:"cloud_model"`
    AutoResolveRisky    bool                 `mapstructure:"auto_resolve_risky"`
    CircuitBreaker      CircuitBreakerConfig `mapstructure:"circuit_breaker"`
    Keepalive           KeepaliveConfig      `mapstructure:"keepalive"`
}
```

**Step 4: Add DefaultAIConfig function**

```go
func DefaultAIConfig() AIConfig {
    return AIConfig{
        Enabled:             false,
        OllamaEndpoint:      "http://localhost:11434",
        Model:               "llama3.2",
        ConfidenceThreshold: 0.8,
        TimeoutSeconds:      5,
        CacheEnabled:        true,
        CircuitBreaker: CircuitBreakerConfig{
            FailureThreshold:     5,
            FailureWindowSeconds: 120,
            CooldownSeconds:      30,
        },
        Keepalive: KeepaliveConfig{
            Enabled:            true,
            IntervalSeconds:    300,
            IdleTimeoutSeconds: 1800,
        },
    }
}
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/config/... -run TestAIConfig_CircuitBreakerDefaults -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(config): add circuit breaker and keepalive config to AIConfig"
```

---

## Phase 2: Circuit Breaker

### Task 4: Create Circuit Breaker Types

**Files:**
- Create: `internal/ai/circuit_breaker.go`
- Create: `internal/ai/circuit_breaker_test.go`

**Step 1: Write failing test**

Create `internal/ai/circuit_breaker_test.go`:

```go
package ai

import (
    "testing"
    "time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
    cb := NewCircuitBreaker(5, 2*time.Minute, 30*time.Second)

    if cb.State() != CircuitClosed {
        t.Errorf("expected initial state CircuitClosed, got %v", cb.State())
    }
    if !cb.Allow() {
        t.Error("expected Allow() to return true when circuit is closed")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/... -run TestCircuitBreaker_InitialState -v
```

Expected: FAIL (types don't exist)

**Step 3: Create circuit_breaker.go with types**

Create `internal/ai/circuit_breaker.go`:

```go
package ai

import (
    "sync"
    "time"
)

type CircuitState int

const (
    CircuitClosed   CircuitState = iota // Normal operation
    CircuitOpen                         // Rejecting requests
    CircuitHalfOpen                     // Testing recovery
)

func (s CircuitState) String() string {
    switch s {
    case CircuitClosed:
        return "closed"
    case CircuitOpen:
        return "open"
    case CircuitHalfOpen:
        return "half-open"
    default:
        return "unknown"
    }
}

type CircuitBreaker struct {
    mu sync.RWMutex

    state      CircuitState
    failures   []time.Time // timestamps of recent failures
    openedAt   time.Time
    lastError  string

    // Configuration
    failureThreshold int
    failureWindow    time.Duration
    cooldownPeriod   time.Duration
}

func NewCircuitBreaker(threshold int, window, cooldown time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        state:            CircuitClosed,
        failures:         make([]time.Time, 0, threshold),
        failureThreshold: threshold,
        failureWindow:    window,
        cooldownPeriod:   cooldown,
    }
}

func (cb *CircuitBreaker) State() CircuitState {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    switch cb.state {
    case CircuitClosed:
        return true
    case CircuitOpen:
        // Check if cooldown has elapsed
        if time.Since(cb.openedAt) >= cb.cooldownPeriod {
            cb.state = CircuitHalfOpen
            return true // Allow one test request
        }
        return false
    case CircuitHalfOpen:
        return true // Allow test request
    default:
        return false
    }
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/ai/... -run TestCircuitBreaker_InitialState -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): add circuit breaker types and initial state"
```

---

### Task 5: Implement Circuit Breaker State Transitions

**Files:**
- Modify: `internal/ai/circuit_breaker.go`
- Modify: `internal/ai/circuit_breaker_test.go`

**Step 1: Write failing test for failure recording**

Add to `internal/ai/circuit_breaker_test.go`:

```go
func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
    cb := NewCircuitBreaker(3, 1*time.Minute, 100*time.Millisecond)

    // Record 3 failures
    cb.RecordFailure("error 1")
    cb.RecordFailure("error 2")
    cb.RecordFailure("error 3")

    if cb.State() != CircuitOpen {
        t.Errorf("expected CircuitOpen after 3 failures, got %v", cb.State())
    }
    if cb.Allow() {
        t.Error("expected Allow() to return false when circuit is open")
    }
}

func TestCircuitBreaker_RecoverAfterCooldown(t *testing.T) {
    cb := NewCircuitBreaker(2, 1*time.Minute, 50*time.Millisecond)

    cb.RecordFailure("error 1")
    cb.RecordFailure("error 2")

    if cb.State() != CircuitOpen {
        t.Fatalf("expected CircuitOpen, got %v", cb.State())
    }

    // Wait for cooldown
    time.Sleep(60 * time.Millisecond)

    // Should transition to half-open on Allow()
    if !cb.Allow() {
        t.Error("expected Allow() to return true after cooldown")
    }
    if cb.State() != CircuitHalfOpen {
        t.Errorf("expected CircuitHalfOpen, got %v", cb.State())
    }

    // Record success
    cb.RecordSuccess()

    if cb.State() != CircuitClosed {
        t.Errorf("expected CircuitClosed after success, got %v", cb.State())
    }
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/ai/... -run TestCircuitBreaker -v
```

Expected: FAIL (RecordFailure, RecordSuccess don't exist)

**Step 3: Implement RecordFailure and RecordSuccess**

Add to `internal/ai/circuit_breaker.go`:

```go
func (cb *CircuitBreaker) RecordFailure(errMsg string) {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    now := time.Now()
    cb.lastError = errMsg

    // Remove old failures outside the window
    cutoff := now.Add(-cb.failureWindow)
    validFailures := make([]time.Time, 0, len(cb.failures))
    for _, t := range cb.failures {
        if t.After(cutoff) {
            validFailures = append(validFailures, t)
        }
    }

    // Add new failure
    validFailures = append(validFailures, now)
    cb.failures = validFailures

    // Check threshold
    if len(cb.failures) >= cb.failureThreshold {
        cb.state = CircuitOpen
        cb.openedAt = now
    }
}

func (cb *CircuitBreaker) RecordSuccess() {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    cb.state = CircuitClosed
    cb.failures = cb.failures[:0] // Clear failures
    cb.lastError = ""
}

func (cb *CircuitBreaker) LastError() string {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return cb.lastError
}

func (cb *CircuitBreaker) CooldownRemaining() time.Duration {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    if cb.state != CircuitOpen {
        return 0
    }

    remaining := cb.cooldownPeriod - time.Since(cb.openedAt)
    if remaining < 0 {
        return 0
    }
    return remaining
}

func (cb *CircuitBreaker) FailureCount() int {
    cb.mu.RLock()
    defer cb.mu.RUnlock()
    return len(cb.failures)
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/ai/... -run TestCircuitBreaker -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): implement circuit breaker state transitions"
```

---

### Task 6: Integrate Circuit Breaker into Integrator

**Files:**
- Modify: `internal/ai/integrator.go`
- Modify: `internal/ai/integrator_test.go`

**Step 1: Write failing test**

Add to `internal/ai/integrator_test.go`:

```go
func TestIntegrator_CircuitBreaker_BlocksWhenOpen(t *testing.T) {
    cfg := config.AIConfig{
        Enabled:             true,
        OllamaEndpoint:      "http://localhost:11434",
        Model:               "test-model",
        ConfidenceThreshold: 0.8,
        TimeoutSeconds:      1,
        CircuitBreaker: config.CircuitBreakerConfig{
            FailureThreshold:     2,
            FailureWindowSeconds: 60,
            CooldownSeconds:      1,
        },
    }

    integrator, err := NewIntegrator(cfg, nil)
    if err != nil {
        t.Fatalf("failed to create integrator: %v", err)
    }
    defer integrator.Close()

    // Simulate failures to open circuit
    integrator.circuit.RecordFailure("test error 1")
    integrator.circuit.RecordFailure("test error 2")

    if integrator.circuit.State() != CircuitOpen {
        t.Fatalf("expected circuit open, got %v", integrator.circuit.State())
    }

    // Should return regex result without calling AI
    title, source, err := integrator.EnhanceTitle("Test Title", "test.file.mkv", "movie")
    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }
    if source != SourceRegex {
        t.Errorf("expected SourceRegex when circuit open, got %v", source)
    }
    if title != "Test Title" {
        t.Errorf("expected original title, got %s", title)
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/... -run TestIntegrator_CircuitBreaker -v
```

Expected: FAIL (circuit field doesn't exist on Integrator)

**Step 3: Add circuit breaker to Integrator struct**

Modify `internal/ai/integrator.go`:

Add field to struct:
```go
type Integrator struct {
    enabled         bool
    matcher         *Matcher
    cache           *Cache
    regexConfidence *ConfidenceCalculator
    config          config.AIConfig  // Update type
    metrics         *Metrics
    circuit         *CircuitBreaker  // NEW

    sfGroup    singleflight.Group
    aiSem      chan struct{}
    warmWg     sync.WaitGroup
    warmSem    chan struct{}
    shutdownCh chan struct{}
}
```

Update NewIntegrator to create circuit breaker:
```go
func NewIntegrator(cfg config.AIConfig, db interface{}) (*Integrator, error) {
    shutdownCh := make(chan struct{})

    // Create circuit breaker
    circuit := NewCircuitBreaker(
        cfg.CircuitBreaker.FailureThreshold,
        time.Duration(cfg.CircuitBreaker.FailureWindowSeconds)*time.Second,
        time.Duration(cfg.CircuitBreaker.CooldownSeconds)*time.Second,
    )

    if !cfg.Enabled {
        return &Integrator{
            enabled:         false,
            regexConfidence: NewConfidenceCalculator(),
            metrics:         &Metrics{},
            circuit:         circuit,
            shutdownCh:      shutdownCh,
        }, nil
    }
    // ... rest of function, add circuit to return
}
```

**Step 4: Update enhanceTitleInternal to check circuit**

In `enhanceTitleInternal`, before AI call:
```go
// Check circuit breaker
if !i.circuit.Allow() {
    i.metrics.RecordParse(SourceRegex, 0)
    return &parseResult{regexTitle, SourceRegex}, nil
}
```

After AI failure, record it:
```go
if err != nil {
    i.circuit.RecordFailure(err.Error())
    i.metrics.RecordAIError()
    // ... rest
}
```

After AI success:
```go
i.circuit.RecordSuccess()
```

**Step 5: Run test to verify it passes**

```bash
go test ./internal/ai/... -run TestIntegrator_CircuitBreaker -v
```

Expected: PASS

**Step 6: Run all AI tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add -A && git commit -m "feat(ai): integrate circuit breaker into Integrator"
```

---

## Phase 3: Response Recovery Chain

### Task 7: Create Recovery Module

**Files:**
- Create: `internal/ai/recovery.go`
- Create: `internal/ai/recovery_test.go`

**Step 1: Write failing test for JSON extraction**

Create `internal/ai/recovery_test.go`:

```go
package ai

import "testing"

func TestExtractPartialResult_ValidJSON(t *testing.T) {
    response := `{"title": "The Matrix", "year": 1999, "type": "movie", "confidence": 0.95}`

    result, ok := ExtractPartialResult(response)
    if !ok {
        t.Fatal("expected extraction to succeed")
    }
    if result.Title != "The Matrix" {
        t.Errorf("expected title 'The Matrix', got '%s'", result.Title)
    }
    if result.Year == nil || *result.Year != 1999 {
        t.Errorf("expected year 1999, got %v", result.Year)
    }
}

func TestExtractPartialResult_BrokenJSON(t *testing.T) {
    // Missing closing brace, malformed
    response := `{"title": "The Matrix", "year": 1999, "type": "movie"`

    result, ok := ExtractPartialResult(response)
    if !ok {
        t.Fatal("expected partial extraction to succeed")
    }
    if result.Title != "The Matrix" {
        t.Errorf("expected title 'The Matrix', got '%s'", result.Title)
    }
}

func TestExtractPartialResult_GarbageResponse(t *testing.T) {
    response := `I'm sorry, I can't help with that request.`

    _, ok := ExtractPartialResult(response)
    if ok {
        t.Error("expected extraction to fail on garbage response")
    }
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ai/... -run TestExtractPartialResult -v
```

Expected: FAIL (function doesn't exist)

**Step 3: Implement ExtractPartialResult**

Create `internal/ai/recovery.go`:

```go
package ai

import (
    "encoding/json"
    "regexp"
    "strconv"
    "strings"
)

var (
    titlePattern = regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`)
    yearPattern  = regexp.MustCompile(`"year"\s*:\s*(\d{4})`)
    typePattern  = regexp.MustCompile(`"type"\s*:\s*"(movie|tv)"`)
    confPattern  = regexp.MustCompile(`"confidence"\s*:\s*([\d.]+)`)
)

// ExtractPartialResult attempts to extract data from malformed JSON responses.
// Returns partial result with lower confidence if extraction succeeds.
func ExtractPartialResult(response string) (*Result, bool) {
    // First try normal JSON parsing
    var result Result
    if err := json.Unmarshal([]byte(response), &result); err == nil {
        return &result, true
    }

    // Try to extract title (required)
    titleMatch := titlePattern.FindStringSubmatch(response)
    if titleMatch == nil {
        return nil, false
    }

    result = Result{
        Title:      titleMatch[1],
        Confidence: 0.7, // Lower confidence for partial extraction
    }

    // Try to extract optional fields
    if yearMatch := yearPattern.FindStringSubmatch(response); yearMatch != nil {
        if year, err := strconv.Atoi(yearMatch[1]); err == nil {
            result.Year = &year
        }
    }

    if typeMatch := typePattern.FindStringSubmatch(response); typeMatch != nil {
        result.Type = typeMatch[1]
    }

    if confMatch := confPattern.FindStringSubmatch(response); confMatch != nil {
        if conf, err := strconv.ParseFloat(confMatch[1], 64); err == nil {
            // Use extracted confidence but cap it since response was malformed
            result.Confidence = min(conf, 0.8)
        }
    }

    return &result, true
}

func min(a, b float64) float64 {
    if a < b {
        return a
    }
    return b
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/ai/... -run TestExtractPartialResult -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): add response recovery with partial extraction"
```

---

### Task 8: Add Retry with Nudge

**Files:**
- Modify: `internal/ai/recovery.go`
- Modify: `internal/ai/recovery_test.go`
- Modify: `internal/ai/matcher.go`

**Step 1: Write failing test**

Add to `internal/ai/recovery_test.go`:

```go
func TestGetNudgePrompt(t *testing.T) {
    prompt := GetNudgePrompt()

    if !strings.Contains(prompt, "valid JSON") {
        t.Error("nudge prompt should mention valid JSON")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/ai/... -run TestGetNudgePrompt -v
```

Expected: FAIL

**Step 3: Add nudge prompt**

Add to `internal/ai/recovery.go`:

```go
// GetNudgePrompt returns the corrective prompt for retry attempts.
func GetNudgePrompt() string {
    return `Your previous response was not valid JSON. Return ONLY valid JSON with no markdown, no explanation, no code blocks. Just the raw JSON object like: {"title": "...", "year": ..., "type": "...", "confidence": ...}`
}
```

**Step 4: Run test**

```bash
go test ./internal/ai/... -run TestGetNudgePrompt -v
```

Expected: PASS

**Step 5: Add ParseWithRetry to Matcher**

Add to `internal/ai/matcher.go`:

```go
// ParseWithRetry attempts to parse, with one retry using a nudge prompt if first attempt fails.
func (m *Matcher) ParseWithRetry(ctx context.Context, filename string) (*Result, error) {
    // First attempt
    result, err := m.Parse(ctx, filename)
    if err == nil {
        return result, nil
    }

    // Check if it was a parse error (not connection error)
    if !strings.Contains(err.Error(), "failed to parse AI response") {
        return nil, err
    }

    // Retry with nudge
    nudgedPrompt := m.systemPrompt + "\n" + filename + "\n\n" + GetNudgePrompt()

    reqBody := GenerateRequest{
        Model:  m.config.Model,
        Prompt: nudgedPrompt,
        Stream: false,
    }

    // ... same request logic as Parse but with nudged prompt
    // (refactor to share code)

    return m.parseWithPrompt(ctx, nudgedPrompt, m.config.Model)
}
```

**Step 6: Refactor parseWithModel to support custom prompts**

The existing `parseWithModel` constructs the prompt internally. Refactor to accept prompt as parameter or add `parseWithPrompt` variant.

**Step 7: Run all tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add -A && git commit -m "feat(ai): add retry with nudge prompt for malformed responses"
```

---

### Task 9: Integrate Recovery Chain into Integrator

**Files:**
- Modify: `internal/ai/integrator.go`

**Step 1: Update enhanceTitleInternal to use recovery chain**

Replace the simple AI call with the recovery chain:

```go
func (i *Integrator) enhanceTitleInternal(regexTitle, filename, mediaType string) (*parseResult, error) {
    normalized := NormalizeForCache(filename)

    // 1. Check cache
    if i.cache != nil {
        cached, err := i.cache.Get(normalized, mediaType, i.config.Model)
        if err == nil && cached != nil {
            i.metrics.RecordParse(SourceCache, 0)
            return &parseResult{cached.Title, SourceCache}, nil
        }
    }

    // 2. Calculate regex confidence
    var regexConf float64
    if mediaType == "tv" {
        regexConf = i.regexConfidence.CalculateTV(regexTitle, filename)
    } else {
        regexConf = i.regexConfidence.CalculateMovie(regexTitle, filename)
    }

    // 3. High confidence - use regex
    if regexConf >= i.config.ConfidenceThreshold {
        i.metrics.RecordParse(SourceRegex, 0)
        i.scheduleWarmCache(normalized, mediaType, regexTitle)
        return &parseResult{regexTitle, SourceRegex}, nil
    }

    // 4. Check circuit breaker
    if !i.circuit.Allow() {
        i.metrics.RecordParse(SourceRegex, 0)
        return &parseResult{regexTitle, SourceRegex}, nil
    }

    // 5. Try AI with recovery chain
    select {
    case i.aiSem <- struct{}{}:
        defer func() { <-i.aiSem }()
    case <-i.shutdownCh:
        i.metrics.RecordParse(SourceRegex, 0)
        return &parseResult{regexTitle, SourceRegex}, nil
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i.config.TimeoutSeconds)*time.Second)
    defer cancel()

    start := time.Now()
    aiResult, err := i.tryAIWithRecovery(ctx, filename)
    latency := time.Since(start)

    if err != nil {
        i.circuit.RecordFailure(err.Error())
        i.metrics.RecordAIError()
        i.metrics.RecordAIFallback()
        i.metrics.RecordParse(SourceRegex, latency)
        return &parseResult{regexTitle, SourceRegex}, nil
    }

    if aiResult.Confidence < i.config.ConfidenceThreshold {
        i.metrics.RecordAIFallback()
        i.metrics.RecordParse(SourceRegex, latency)
        return &parseResult{regexTitle, SourceRegex}, nil
    }

    // Success
    i.circuit.RecordSuccess()

    // Cache result
    if i.cache != nil {
        _ = i.cache.Put(normalized, mediaType, i.config.Model, aiResult, latency)
    }

    i.metrics.RecordParse(SourceAI, latency)
    return &parseResult{aiResult.Title, SourceAI}, nil
}

func (i *Integrator) tryAIWithRecovery(ctx context.Context, filename string) (*Result, error) {
    // Step 1: Try normal parse
    result, err := i.matcher.Parse(ctx, filename)
    if err == nil {
        return result, nil
    }

    // Step 2: Try to extract partial from error response
    if strings.Contains(err.Error(), "response:") {
        // Extract the response part from error
        parts := strings.SplitN(err.Error(), "response:", 2)
        if len(parts) == 2 {
            if partial, ok := ExtractPartialResult(strings.TrimSpace(parts[1])); ok {
                return partial, nil
            }
        }
    }

    // Step 3: Retry with nudge (if not a connection error)
    if !isConnectionError(err) {
        result, retryErr := i.matcher.ParseWithRetry(ctx, filename)
        if retryErr == nil {
            return result, nil
        }
    }

    return nil, err
}

func isConnectionError(err error) bool {
    errStr := err.Error()
    return strings.Contains(errStr, "connection refused") ||
           strings.Contains(errStr, "no such host") ||
           strings.Contains(errStr, "timeout")
}
```

**Step 2: Run all tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add -A && git commit -m "feat(ai): integrate recovery chain into Integrator"
```

---

## Phase 4: Background Enhancement Queue

### Task 10: Create Background Queue Types

**Files:**
- Create: `internal/ai/background.go`
- Create: `internal/ai/background_test.go`

**Step 1: Write failing test**

Create `internal/ai/background_test.go`:

```go
package ai

import (
    "testing"
    "time"
)

func TestBackgroundQueue_Submit(t *testing.T) {
    q := NewBackgroundQueue(10)
    defer q.Close()

    req := &EnhancementRequest{
        Filename:   "test.mkv",
        RegexTitle: "Test",
        MediaType:  "movie",
    }

    if !q.Submit(req) {
        t.Error("expected submit to succeed")
    }

    if q.Pending() != 1 {
        t.Errorf("expected 1 pending, got %d", q.Pending())
    }
}

func TestBackgroundQueue_DropWhenFull(t *testing.T) {
    q := NewBackgroundQueue(2)
    defer q.Close()

    q.Submit(&EnhancementRequest{Filename: "1.mkv"})
    q.Submit(&EnhancementRequest{Filename: "2.mkv"})

    // Third should be dropped (non-blocking)
    if q.Submit(&EnhancementRequest{Filename: "3.mkv"}) {
        t.Error("expected submit to fail when queue full")
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/ai/... -run TestBackgroundQueue -v
```

Expected: FAIL

**Step 3: Implement BackgroundQueue**

Create `internal/ai/background.go`:

```go
package ai

import (
    "sync"
    "sync/atomic"
)

type EnhancementRequest struct {
    Filename   string
    RegexTitle string
    MediaType  string
    FilePath   string // Actual file path for potential rename
}

type EnhancementResult struct {
    Request     *EnhancementRequest
    AITitle     string
    Confidence  float64
    Improved    bool // True if AI title significantly differs from regex
    Error       error
}

type BackgroundQueue struct {
    queue    chan *EnhancementRequest
    pending  int64
    closed   bool
    closeMu  sync.RWMutex
}

func NewBackgroundQueue(size int) *BackgroundQueue {
    return &BackgroundQueue{
        queue: make(chan *EnhancementRequest, size),
    }
}

func (q *BackgroundQueue) Submit(req *EnhancementRequest) bool {
    q.closeMu.RLock()
    defer q.closeMu.RUnlock()

    if q.closed {
        return false
    }

    select {
    case q.queue <- req:
        atomic.AddInt64(&q.pending, 1)
        return true
    default:
        return false // Queue full, drop request
    }
}

func (q *BackgroundQueue) Receive() <-chan *EnhancementRequest {
    return q.queue
}

func (q *BackgroundQueue) MarkProcessed() {
    atomic.AddInt64(&q.pending, -1)
}

func (q *BackgroundQueue) Pending() int {
    return int(atomic.LoadInt64(&q.pending))
}

func (q *BackgroundQueue) Close() {
    q.closeMu.Lock()
    defer q.closeMu.Unlock()

    if !q.closed {
        q.closed = true
        close(q.queue)
    }
}
```

**Step 4: Run test**

```bash
go test ./internal/ai/... -run TestBackgroundQueue -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): add background enhancement queue"
```

---

### Task 11: Add Background Worker to Integrator

**Files:**
- Modify: `internal/ai/integrator.go`

**Step 1: Add queue and worker to Integrator**

Add field:
```go
type Integrator struct {
    // ... existing fields
    bgQueue    *BackgroundQueue
    bgWg       sync.WaitGroup
}
```

**Step 2: Start worker in NewIntegrator**

```go
func NewIntegrator(cfg config.AIConfig, db interface{}) (*Integrator, error) {
    // ... existing code

    i := &Integrator{
        // ... existing fields
        bgQueue: NewBackgroundQueue(100),
    }

    // Start background worker
    i.bgWg.Add(1)
    go i.backgroundWorker()

    return i, nil
}

func (i *Integrator) backgroundWorker() {
    defer i.bgWg.Done()

    for req := range i.bgQueue.Receive() {
        i.processBackgroundRequest(req)
        i.bgQueue.MarkProcessed()
    }
}

func (i *Integrator) processBackgroundRequest(req *EnhancementRequest) {
    if !i.circuit.Allow() {
        return // Circuit open, skip
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i.config.TimeoutSeconds)*time.Second)
    defer cancel()

    result, err := i.tryAIWithRecovery(ctx, req.Filename)
    if err != nil {
        i.circuit.RecordFailure(err.Error())
        return
    }

    i.circuit.RecordSuccess()

    // Cache the result
    if i.cache != nil {
        normalized := NormalizeForCache(req.Filename)
        _ = i.cache.Put(normalized, req.MediaType, i.config.Model, result, 0)
    }

    // Check if significantly improved
    if result.Title != req.RegexTitle && result.Confidence >= i.config.ConfidenceThreshold {
        // TODO: Queue for review (Task 12)
    }
}
```

**Step 3: Update Close to wait for worker**

```go
func (i *Integrator) Close() error {
    close(i.shutdownCh)
    i.warmWg.Wait()

    if i.bgQueue != nil {
        i.bgQueue.Close()
    }
    i.bgWg.Wait()

    return nil
}
```

**Step 4: Add QueueForEnhancement method**

```go
// QueueForEnhancement submits a file for background AI processing.
// This is non-blocking and will drop the request if the queue is full.
func (i *Integrator) QueueForEnhancement(filename, regexTitle, mediaType, filePath string) bool {
    if !i.enabled || i.bgQueue == nil {
        return false
    }

    return i.bgQueue.Submit(&EnhancementRequest{
        Filename:   filename,
        RegexTitle: regexTitle,
        MediaType:  mediaType,
        FilePath:   filePath,
    })
}
```

**Step 5: Run tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(ai): add background worker for async enhancement"
```

---

## Phase 5: AI Improvements Queue

### Task 12: Add Database Migration for ai_improvements Table

**Files:**
- Modify: `internal/database/schema.go`

**Step 1: Find current schema version**

```bash
grep -n "schema_version" internal/database/schema.go | tail -5
```

**Step 2: Add new migration**

Add new migration to the migrations slice:

```go
// Migration N: AI improvements table
{
    `CREATE TABLE ai_improvements (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        file_path TEXT NOT NULL,
        current_title TEXT NOT NULL,
        suggested_title TEXT NOT NULL,
        current_year TEXT,
        suggested_year TEXT,
        media_type TEXT NOT NULL CHECK(media_type IN ('movie', 'tv')),
        regex_confidence REAL NOT NULL,
        ai_confidence REAL NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'approved', 'rejected', 'ignored')),
        reviewed_at DATETIME,
        UNIQUE(file_path)
    )`,
    `CREATE INDEX idx_ai_improvements_status ON ai_improvements(status)`,
    `INSERT INTO schema_version (version) VALUES (N)`,
},
```

**Step 3: Run database tests**

```bash
go test ./internal/database/... -v
```

Expected: PASS

**Step 4: Commit**

```bash
git add -A && git commit -m "feat(db): add ai_improvements table migration"
```

---

### Task 13: Create Improvements Repository

**Files:**
- Create: `internal/ai/improvements.go`
- Create: `internal/ai/improvements_test.go`

**Step 1: Write failing test**

Create `internal/ai/improvements_test.go`:

```go
package ai

import (
    "database/sql"
    "os"
    "testing"

    _ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open db: %v", err)
    }

    _, err = db.Exec(`CREATE TABLE ai_improvements (
        id INTEGER PRIMARY KEY,
        file_path TEXT NOT NULL UNIQUE,
        current_title TEXT NOT NULL,
        suggested_title TEXT NOT NULL,
        current_year TEXT,
        suggested_year TEXT,
        media_type TEXT NOT NULL,
        regex_confidence REAL NOT NULL,
        ai_confidence REAL NOT NULL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        status TEXT DEFAULT 'pending',
        reviewed_at DATETIME
    )`)
    if err != nil {
        t.Fatalf("failed to create table: %v", err)
    }

    return db
}

func TestImprovements_Add(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    repo := NewImprovementsRepo(db)

    err := repo.Add(&Improvement{
        FilePath:        "/media/Movies/Test.mkv",
        CurrentTitle:    "Test",
        SuggestedTitle:  "The Test",
        MediaType:       "movie",
        RegexConfidence: 0.5,
        AIConfidence:    0.95,
    })

    if err != nil {
        t.Fatalf("failed to add improvement: %v", err)
    }

    count, err := repo.PendingCount()
    if err != nil {
        t.Fatalf("failed to count: %v", err)
    }
    if count != 1 {
        t.Errorf("expected 1 pending, got %d", count)
    }
}
```

**Step 2: Run test**

```bash
go test ./internal/ai/... -run TestImprovements -v
```

Expected: FAIL

**Step 3: Implement ImprovementsRepo**

Create `internal/ai/improvements.go`:

```go
package ai

import (
    "database/sql"
    "time"
)

type Improvement struct {
    ID              int
    FilePath        string
    CurrentTitle    string
    SuggestedTitle  string
    CurrentYear     string
    SuggestedYear   string
    MediaType       string
    RegexConfidence float64
    AIConfidence    float64
    CreatedAt       time.Time
    Status          string
    ReviewedAt      *time.Time
}

type ImprovementsRepo struct {
    db *sql.DB
}

func NewImprovementsRepo(db *sql.DB) *ImprovementsRepo {
    return &ImprovementsRepo{db: db}
}

func (r *ImprovementsRepo) Add(imp *Improvement) error {
    _, err := r.db.Exec(`
        INSERT INTO ai_improvements (
            file_path, current_title, suggested_title, current_year, suggested_year,
            media_type, regex_confidence, ai_confidence
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(file_path) DO UPDATE SET
            suggested_title = excluded.suggested_title,
            suggested_year = excluded.suggested_year,
            ai_confidence = excluded.ai_confidence,
            status = 'pending',
            reviewed_at = NULL
    `,
        imp.FilePath, imp.CurrentTitle, imp.SuggestedTitle,
        imp.CurrentYear, imp.SuggestedYear, imp.MediaType,
        imp.RegexConfidence, imp.AIConfidence,
    )
    return err
}

func (r *ImprovementsRepo) PendingCount() (int, error) {
    var count int
    err := r.db.QueryRow(`SELECT COUNT(*) FROM ai_improvements WHERE status = 'pending'`).Scan(&count)
    return count, err
}

func (r *ImprovementsRepo) ListPending(limit int) ([]*Improvement, error) {
    rows, err := r.db.Query(`
        SELECT id, file_path, current_title, suggested_title, current_year, suggested_year,
               media_type, regex_confidence, ai_confidence, created_at, status
        FROM ai_improvements
        WHERE status = 'pending'
        ORDER BY ai_confidence DESC
        LIMIT ?
    `, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var improvements []*Improvement
    for rows.Next() {
        imp := &Improvement{}
        err := rows.Scan(
            &imp.ID, &imp.FilePath, &imp.CurrentTitle, &imp.SuggestedTitle,
            &imp.CurrentYear, &imp.SuggestedYear, &imp.MediaType,
            &imp.RegexConfidence, &imp.AIConfidence, &imp.CreatedAt, &imp.Status,
        )
        if err != nil {
            return nil, err
        }
        improvements = append(improvements, imp)
    }

    return improvements, nil
}

func (r *ImprovementsRepo) UpdateStatus(id int, status string) error {
    _, err := r.db.Exec(`
        UPDATE ai_improvements
        SET status = ?, reviewed_at = CURRENT_TIMESTAMP
        WHERE id = ?
    `, status, id)
    return err
}
```

**Step 4: Run test**

```bash
go test ./internal/ai/... -run TestImprovements -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): add improvements repository for review queue"
```

---

### Task 14: Wire Improvements into Background Worker

**Files:**
- Modify: `internal/ai/integrator.go`

**Step 1: Add improvements repo to Integrator**

```go
type Integrator struct {
    // ... existing
    improvements *ImprovementsRepo
}
```

**Step 2: Initialize in NewIntegrator**

```go
if sqlDB, ok := db.(SQLDatabase); ok {
    cache = NewCache(sqlDB.DB())
    improvements = NewImprovementsRepo(sqlDB.DB())
}
```

**Step 3: Update processBackgroundRequest to queue improvements**

```go
func (i *Integrator) processBackgroundRequest(req *EnhancementRequest) {
    // ... existing code up to checking improvement

    // Check if significantly improved
    if result.Title != req.RegexTitle && result.Confidence >= i.config.ConfidenceThreshold {
        if i.improvements != nil {
            _ = i.improvements.Add(&Improvement{
                FilePath:        req.FilePath,
                CurrentTitle:    req.RegexTitle,
                SuggestedTitle:  result.Title,
                MediaType:       req.MediaType,
                RegexConfidence: 0, // TODO: pass this through
                AIConfidence:    result.Confidence,
            })
        }
    }
}
```

**Step 4: Run tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(ai): wire improvements repo into background worker"
```

---

## Phase 6: Observability

### Task 15: Create AI Status Type

**Files:**
- Create: `internal/ai/status.go`
- Create: `internal/ai/status_test.go`

**Step 1: Write test**

```go
package ai

import "testing"

func TestAIStatus_Healthy(t *testing.T) {
    status := &AIStatus{
        Enabled:      true,
        CircuitState: CircuitClosed,
        ModelWarm:    true,
    }

    if !status.IsHealthy() {
        t.Error("expected healthy status")
    }
}

func TestAIStatus_Degraded(t *testing.T) {
    status := &AIStatus{
        Enabled:      true,
        CircuitState: CircuitOpen,
    }

    if status.IsHealthy() {
        t.Error("expected degraded status")
    }
    if !status.IsDegraded() {
        t.Error("expected IsDegraded to return true")
    }
}
```

**Step 2: Implement status.go**

```go
package ai

import "time"

type AIStatus struct {
    Enabled           bool
    CircuitState      CircuitState
    CircuitOpenReason string
    CooldownRemaining time.Duration
    LastSuccess       time.Time
    ModelWarm         bool
    Model             string
    CacheEntries      int
    CacheHitRate      float64
    PendingReviews    int
}

func (s *AIStatus) IsHealthy() bool {
    return s.Enabled && s.CircuitState == CircuitClosed
}

func (s *AIStatus) IsDegraded() bool {
    return s.Enabled && s.CircuitState != CircuitClosed
}

func (s *AIStatus) StatusString() string {
    if !s.Enabled {
        return "disabled"
    }
    switch s.CircuitState {
    case CircuitClosed:
        return "enabled (circuit closed)"
    case CircuitOpen:
        return "degraded (circuit open)"
    case CircuitHalfOpen:
        return "recovering (circuit half-open)"
    default:
        return "unknown"
    }
}
```

**Step 3: Add Status() method to Integrator**

```go
func (i *Integrator) Status() *AIStatus {
    status := &AIStatus{
        Enabled: i.enabled,
        Model:   i.config.Model,
    }

    if !i.enabled {
        return status
    }

    status.CircuitState = i.circuit.State()
    status.CircuitOpenReason = i.circuit.LastError()
    status.CooldownRemaining = i.circuit.CooldownRemaining()
    status.LastSuccess = i.metrics.LastSuccess()

    if i.cache != nil {
        entries, hitRate, _ := i.cache.GetStats()
        status.CacheEntries = int(entries)
        status.CacheHitRate = float64(hitRate)
    }

    if i.improvements != nil {
        count, _ := i.improvements.PendingCount()
        status.PendingReviews = count
    }

    return status
}
```

**Step 4: Add LastSuccess tracking to Metrics**

Update `internal/ai/types.go`:

```go
type Metrics struct {
    // ... existing
    lastSuccess atomic.Value // time.Time
}

func (m *Metrics) RecordSuccess() {
    m.lastSuccess.Store(time.Now())
}

func (m *Metrics) LastSuccess() time.Time {
    if v := m.lastSuccess.Load(); v != nil {
        return v.(time.Time)
    }
    return time.Time{}
}
```

**Step 5: Run tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(ai): add AIStatus for observability"
```

---

## Phase 7: Keepalive

### Task 16: Implement Keepalive

**Files:**
- Create: `internal/ai/keepalive.go`
- Create: `internal/ai/keepalive_test.go`

**Step 1: Write test**

```go
package ai

import (
    "testing"
    "time"
    "sync/atomic"
)

func TestKeepalive_PingsWhenActive(t *testing.T) {
    var pingCount int64

    k := NewKeepalive(50*time.Millisecond, 200*time.Millisecond, func() {
        atomic.AddInt64(&pingCount, 1)
    })

    k.RecordActivity()
    k.Start()
    defer k.Stop()

    time.Sleep(120 * time.Millisecond)

    if atomic.LoadInt64(&pingCount) < 1 {
        t.Error("expected at least one ping")
    }
}

func TestKeepalive_NoPingWhenIdle(t *testing.T) {
    var pingCount int64

    k := NewKeepalive(50*time.Millisecond, 10*time.Millisecond, func() {
        atomic.AddInt64(&pingCount, 1)
    })

    // Don't record activity - should be idle
    k.Start()
    defer k.Stop()

    time.Sleep(80 * time.Millisecond)

    if atomic.LoadInt64(&pingCount) > 0 {
        t.Error("expected no pings when idle")
    }
}
```

**Step 2: Implement keepalive.go**

```go
package ai

import (
    "sync"
    "time"
)

type Keepalive struct {
    interval    time.Duration
    idleTimeout time.Duration
    pingFunc    func()

    lastActivity time.Time
    mu           sync.RWMutex
    ticker       *time.Ticker
    stopCh       chan struct{}
    running      bool
}

func NewKeepalive(interval, idleTimeout time.Duration, pingFunc func()) *Keepalive {
    return &Keepalive{
        interval:    interval,
        idleTimeout: idleTimeout,
        pingFunc:    pingFunc,
        stopCh:      make(chan struct{}),
    }
}

func (k *Keepalive) Start() {
    k.mu.Lock()
    if k.running {
        k.mu.Unlock()
        return
    }
    k.running = true
    k.ticker = time.NewTicker(k.interval)
    k.mu.Unlock()

    go func() {
        for {
            select {
            case <-k.ticker.C:
                k.mu.RLock()
                shouldPing := !k.lastActivity.IsZero() &&
                              time.Since(k.lastActivity) < k.idleTimeout
                k.mu.RUnlock()

                if shouldPing && k.pingFunc != nil {
                    k.pingFunc()
                }
            case <-k.stopCh:
                return
            }
        }
    }()
}

func (k *Keepalive) Stop() {
    k.mu.Lock()
    defer k.mu.Unlock()

    if !k.running {
        return
    }

    k.running = false
    if k.ticker != nil {
        k.ticker.Stop()
    }
    close(k.stopCh)
}

func (k *Keepalive) RecordActivity() {
    k.mu.Lock()
    k.lastActivity = time.Now()
    k.mu.Unlock()
}
```

**Step 3: Run tests**

```bash
go test ./internal/ai/... -run TestKeepalive -v
```

Expected: PASS

**Step 4: Wire into Integrator**

Add to NewIntegrator:
```go
if cfg.Keepalive.Enabled {
    i.keepalive = NewKeepalive(
        time.Duration(cfg.Keepalive.IntervalSeconds)*time.Second,
        time.Duration(cfg.Keepalive.IdleTimeoutSeconds)*time.Second,
        func() { i.matcher.Ping(context.Background()) },
    )
    i.keepalive.Start()
}
```

Add Ping to matcher:
```go
func (m *Matcher) Ping(ctx context.Context) error {
    // Minimal request to keep model warm
    req, _ := http.NewRequestWithContext(ctx, "GET", m.config.OllamaEndpoint+"/api/tags", nil)
    resp, err := m.client.Do(req)
    if err != nil {
        return err
    }
    resp.Body.Close()
    return nil
}
```

**Step 5: Update Close**

```go
if i.keepalive != nil {
    i.keepalive.Stop()
}
```

**Step 6: Run all tests**

```bash
go test ./internal/ai/... -v
```

Expected: PASS

**Step 7: Commit**

```bash
git add -A && git commit -m "feat(ai): add keepalive to prevent model cold starts"
```

---

## Phase 8: Integration Tests

### Task 17: Add Integration Test for Full Flow

**Files:**
- Create: `internal/ai/integration_test.go`

**Step 1: Write integration test**

```go
//go:build integration

package ai

import (
    "testing"
    "time"

    "github.com/Nomadcxx/jellywatch/internal/config"
)

func TestIntegration_FullFlow(t *testing.T) {
    cfg := config.AIConfig{
        Enabled:             true,
        OllamaEndpoint:      "http://localhost:11434",
        Model:               "llama3.2",
        ConfidenceThreshold: 0.8,
        TimeoutSeconds:      10,
        CacheEnabled:        true,
        CircuitBreaker: config.CircuitBreakerConfig{
            FailureThreshold:     5,
            FailureWindowSeconds: 120,
            CooldownSeconds:      30,
        },
    }

    integrator, err := NewIntegrator(cfg, nil)
    if err != nil {
        t.Fatalf("failed to create integrator: %v", err)
    }
    defer integrator.Close()

    // Test with known filename
    title, source, err := integrator.EnhanceTitle(
        "Matrix",
        "The.Matrix.1999.2160p.UHD.BluRay.x265-GROUP.mkv",
        "movie",
    )

    if err != nil {
        t.Errorf("unexpected error: %v", err)
    }

    t.Logf("Title: %s, Source: %v", title, source)

    // Verify status
    status := integrator.Status()
    t.Logf("Status: %+v", status)
}
```

**Step 2: Run integration test (requires Ollama)**

```bash
go test ./internal/ai/... -tags=integration -run TestIntegration -v
```

**Step 3: Commit**

```bash
git add -A && git commit -m "test(ai): add integration test for full flow"
```

---

## Summary

This implementation plan covers:

1. **Code Cleanup** (Tasks 1-3): Remove dead code, consolidate config
2. **Circuit Breaker** (Tasks 4-6): Types, state transitions, integration
3. **Response Recovery** (Tasks 7-9): Partial extraction, retry nudge, integration
4. **Background Queue** (Tasks 10-11): Async enhancement processing
5. **Improvements Queue** (Tasks 12-14): Database migration, repository, wiring
6. **Observability** (Task 15): AIStatus type for status display
7. **Keepalive** (Task 16): Cold start prevention
8. **Integration Tests** (Task 17): End-to-end verification

Total: 17 tasks, each with TDD steps (write failing test, implement, verify, commit).
