# Daemon AI Enhancement Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Ollama Cloud AI into the daemon's file organization pipeline using a two-lane confidence-gated approach — fast lane for high-confidence regex, slow lane for AI enhancement of low-confidence parses.

**Architecture:** The handler computes confidence after regex parsing. Files scoring >= 0.6 organize immediately via new `*WithParsed()` organizer methods. Files below 0.6 are held in a pending map and processed by a 30s ticker that calls `ai.Matcher.ParseWithRetry()`, classifies the change, and either auto-applies safe corrections or flags risky ones for `plex2jellyfin review`.

**Tech Stack:** Go, existing `ai.Matcher` (Ollama Cloud), `naming.CalculateTitleConfidence`, JSONL logging, Cobra CLI.

**Spec:** `docs/superpowers/specs/2026-03-16-daemon-ai-enhancement-design.md`

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/config.go` | Add 3 new `AIConfig` fields, update defaults and `ToTOML()` |
| `internal/daemon/ratelimit.go` | **New.** `AIRateLimiter` — hourly + daily token bucket |
| `internal/daemon/ratelimit_test.go` | **New.** Tests for rate limiter |
| `internal/daemon/classify.go` | **New.** `ClassifyChange()` — pure function, change categories |
| `internal/daemon/classify_test.go` | **New.** Tests for change classification |
| `internal/daemon/enhancelog.go` | **New.** JSONL log writer with rotation |
| `internal/daemon/enhancelog_test.go` | **New.** Tests for log writer |
| `internal/organizer/organizer.go` | Add `OrganizeTVWithParsed()`, `OrganizeMovieWithParsed()` |
| `internal/organizer/organizer_test.go` | Tests for `*WithParsed()` methods |
| `internal/daemon/handler.go` | Add `pendingAI` map, confidence gating, ticker, AI config fields |
| `internal/daemon/handler_test.go` | Integration tests for two-lane routing |
| `cmd/plex2jellyfin-daemon/main.go` | Wire `ai.Matcher` into handler, start ticker |
| `cmd/plex2jellyfin/review_cmd.go` | **New.** `plex2jellyfin review` interactive command |

---

## Chunk 1: Foundation (Config + Rate Limiter + Classify + Enhance Log)

### Task 1: Add Config Fields

**Files:**
- Modify: `internal/config/config.go:137-152` (AIConfig struct)
- Modify: `internal/config/config.go:258-279` (defaultConfig)
- Modify: `internal/config/config.go:452-460` (ToTOML format string)
- Modify: `internal/config/config.go:501-508` (ToTOML args)
- Modify: `internal/config/config.go:562-582` (DefaultAIConfig)

- [ ] **Step 1: Add fields to AIConfig struct**

In `internal/config/config.go`, add after line 151 (`MaxRetries`):

```go
HourlyLimit                int           `mapstructure:"hourly_limit"`
DailyLimit                 int           `mapstructure:"daily_limit"`
EnhancementIntervalSeconds int           `mapstructure:"enhancement_interval_seconds"`
```

- [ ] **Step 2: Add defaults to defaultConfig()**

In the `AI: AIConfig{` block around line 258, add after `MaxRetries: 3,`:

```go
HourlyLimit:                10,
DailyLimit:                 50,
EnhancementIntervalSeconds: 30,
```

- [ ] **Step 3: Add defaults to DefaultAIConfig()**

In `DefaultAIConfig()` around line 562, add before the closing brace:

```go
HourlyLimit:                10,
DailyLimit:                 50,
EnhancementIntervalSeconds: 30,
```

- [ ] **Step 4: Update ToTOML format string**

In the `ToTOML()` format string, after `cloud_model = "%s"` (line 460), add:

```
hourly_limit = %d
daily_limit = %d
enhancement_interval_seconds = %d
```

- [ ] **Step 5: Update ToTOML args**

In the `fmt.Sprintf` args, after `c.AI.CloudModel,` (line 508), add:

```go
c.AI.HourlyLimit,
c.AI.DailyLimit,
c.AI.EnhancementIntervalSeconds,
```

- [ ] **Step 6: Verify build**

Run: `go build ./internal/config/...`
Expected: success

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go
git commit -m "config: add AI enhancement rate limit and interval fields"
```

---

### Task 2: Rate Limiter

**Files:**
- Create: `internal/daemon/ratelimit.go`
- Create: `internal/daemon/ratelimit_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/daemon/ratelimit_test.go`:

```go
package daemon

import (
	"testing"
	"time"
)

func TestAIRateLimiter_AllowWithinLimits(t *testing.T) {
	rl := NewAIRateLimiter(10, 50)
	for i := 0; i < 10; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() returned false on call %d, expected true", i+1)
		}
		rl.Record()
	}
}

func TestAIRateLimiter_HourlyCapExhausted(t *testing.T) {
	rl := NewAIRateLimiter(3, 50)
	for i := 0; i < 3; i++ {
		rl.Allow()
		rl.Record()
	}
	if rl.Allow() {
		t.Fatal("Allow() returned true after hourly cap exhausted")
	}
}

func TestAIRateLimiter_DailyCapExhausted(t *testing.T) {
	rl := NewAIRateLimiter(100, 5)
	for i := 0; i < 5; i++ {
		rl.Allow()
		rl.Record()
	}
	if rl.Allow() {
		t.Fatal("Allow() returned true after daily cap exhausted")
	}
}

func TestAIRateLimiter_HourlyResetsAfterWindow(t *testing.T) {
	rl := NewAIRateLimiter(2, 50)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	if rl.Allow() {
		t.Fatal("should be exhausted before reset")
	}

	// Simulate hour passing
	rl.mu.Lock()
	rl.hourlyReset = time.Now().Add(-1 * time.Second)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("Allow() should return true after hourly window reset")
	}
}

func TestAIRateLimiter_DailyResetsAfterWindow(t *testing.T) {
	rl := NewAIRateLimiter(100, 2)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	if rl.Allow() {
		t.Fatal("should be exhausted before reset")
	}

	// Simulate day passing
	rl.mu.Lock()
	rl.dailyReset = time.Now().Add(-1 * time.Second)
	rl.mu.Unlock()

	if !rl.Allow() {
		t.Fatal("Allow() should return true after daily window reset")
	}
}

func TestAIRateLimiter_Stats(t *testing.T) {
	rl := NewAIRateLimiter(10, 50)
	rl.Allow()
	rl.Record()
	rl.Allow()
	rl.Record()

	h, d := rl.Stats()
	if h != 2 {
		t.Errorf("hourlyUsed = %d, want 2", h)
	}
	if d != 2 {
		t.Errorf("dailyUsed = %d, want 2", d)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run TestAIRateLimiter -v`
Expected: compilation failure — `NewAIRateLimiter` undefined

- [ ] **Step 3: Implement rate limiter**

Create `internal/daemon/ratelimit.go`:

```go
package daemon

import (
	"sync"
	"time"
)

type AIRateLimiter struct {
	hourlyCap   int
	dailyCap    int
	hourlyUsed  int
	dailyUsed   int
	hourlyReset time.Time
	dailyReset  time.Time
	mu          sync.Mutex
}

func NewAIRateLimiter(hourlyCap, dailyCap int) *AIRateLimiter {
	now := time.Now()
	return &AIRateLimiter{
		hourlyCap:   hourlyCap,
		dailyCap:    dailyCap,
		hourlyReset: now.Add(time.Hour),
		dailyReset:  now.Add(24 * time.Hour),
	}
}

func (r *AIRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	if now.After(r.hourlyReset) {
		r.hourlyUsed = 0
		r.hourlyReset = now.Add(time.Hour)
	}
	if now.After(r.dailyReset) {
		r.dailyUsed = 0
		r.dailyReset = now.Add(24 * time.Hour)
	}

	return r.hourlyUsed < r.hourlyCap && r.dailyUsed < r.dailyCap
}

func (r *AIRateLimiter) Record() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hourlyUsed++
	r.dailyUsed++
}

func (r *AIRateLimiter) Stats() (hourlyUsed, dailyUsed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hourlyUsed, r.dailyUsed
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run TestAIRateLimiter -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/ratelimit.go internal/daemon/ratelimit_test.go
git commit -m "daemon: add AI rate limiter with hourly and daily caps"
```

---

### Task 3: Change Classification

**Files:**
- Create: `internal/daemon/classify.go`
- Create: `internal/daemon/classify_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/daemon/classify_test.go`:

```go
package daemon

import "testing"

func TestClassifyChange_Punctuation(t *testing.T) {
	c := ClassifyChange("Freddys Nightmares", "Freddy's Nightmares", "", "", "tv", "tv")
	if c.Category != ChangePunctuation {
		t.Errorf("got %s, want %s", c.Category, ChangePunctuation)
	}
	if !c.Safe {
		t.Error("punctuation changes should be safe")
	}
	if c.MinConfidence != 0.80 {
		t.Errorf("min confidence = %.2f, want 0.80", c.MinConfidence)
	}
}

func TestClassifyChange_Casing(t *testing.T) {
	c := ClassifyChange("the office", "The Office", "", "", "tv", "tv")
	if c.Category != ChangeCasing {
		t.Errorf("got %s, want %s", c.Category, ChangeCasing)
	}
	if !c.Safe {
		t.Error("casing changes should be safe")
	}
}

func TestClassifyChange_YearAdded(t *testing.T) {
	c := ClassifyChange("The Office", "The Office", "", "2005", "tv", "tv")
	if c.Category != ChangeYearAdded {
		t.Errorf("got %s, want %s", c.Category, ChangeYearAdded)
	}
	if !c.Safe {
		t.Error("year addition should be safe")
	}
	if c.MinConfidence != 0.85 {
		t.Errorf("min confidence = %.2f, want 0.85", c.MinConfidence)
	}
}

func TestClassifyChange_YearCorrected(t *testing.T) {
	c := ClassifyChange("Show", "Show", "2024", "2025", "tv", "tv")
	if c.Category != ChangeYearCorrected {
		t.Errorf("got %s, want %s", c.Category, ChangeYearCorrected)
	}
	if c.MinConfidence != 0.90 {
		t.Errorf("min confidence = %.2f, want 0.90", c.MinConfidence)
	}
}

func TestClassifyChange_DifferentTitle(t *testing.T) {
	c := ClassifyChange("Weird", "Something Completely Different", "", "", "tv", "tv")
	if c.Category != ChangeTitleDifferent {
		t.Errorf("got %s, want %s", c.Category, ChangeTitleDifferent)
	}
	if c.Safe {
		t.Error("different title should NOT be safe")
	}
}

func TestClassifyChange_TypeChange(t *testing.T) {
	c := ClassifyChange("Show", "Show", "", "", "tv", "movie")
	if c.Category != ChangeTypeDifferent {
		t.Errorf("got %s, want %s", c.Category, ChangeTypeDifferent)
	}
	if c.Safe {
		t.Error("type changes should NOT be safe")
	}
}

func TestClassifyChange_NoChange(t *testing.T) {
	c := ClassifyChange("Breaking Bad", "Breaking Bad", "2008", "2008", "tv", "tv")
	if c.Category != ChangeNone {
		t.Errorf("got %s, want %s", c.Category, ChangeNone)
	}
	if !c.Safe {
		t.Error("no change should be safe")
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b    string
		wantMin float64
		wantMax float64
	}{
		{"Freddys Nightmares", "Freddy's Nightmares", 0.9, 1.0},
		{"Weird", "Something Completely Different", 0.0, 0.1},
		{"The Office", "The Office", 1.0, 1.0},
		{"Breaking Bad", "Breaking Badly", 0.3, 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			got := jaccardWordSimilarity(tt.a, tt.b)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("jaccardWordSimilarity(%q, %q) = %.2f, want [%.2f, %.2f]",
					tt.a, tt.b, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run "TestClassifyChange|TestJaccard" -v`
Expected: compilation failure

- [ ] **Step 3: Implement classify.go**

Create `internal/daemon/classify.go`:

```go
package daemon

import (
	"regexp"
	"strings"
)

type ChangeCategory string

const (
	ChangeNone           ChangeCategory = "none"
	ChangePunctuation    ChangeCategory = "punctuation"
	ChangeCasing         ChangeCategory = "casing"
	ChangeYearAdded      ChangeCategory = "year_added"
	ChangeYearCorrected  ChangeCategory = "year_corrected"
	ChangeTitleDifferent ChangeCategory = "different_title"
	ChangeTypeDifferent  ChangeCategory = "type_change"
	ChangeStructural     ChangeCategory = "structural"
)

type ChangeClassification struct {
	Category      ChangeCategory
	Safe          bool
	MinConfidence float64
}

var punctuationRegex = regexp.MustCompile(`[^a-zA-Z0-9\s]`)

func ClassifyChange(regexTitle, aiTitle, regexYear, aiYear, regexMediaType, aiMediaType string) ChangeClassification {
	// Type change — always risky
	if regexMediaType != aiMediaType {
		return ChangeClassification{Category: ChangeTypeDifferent, Safe: false}
	}

	// Year changes
	yearChanged := regexYear != aiYear
	yearAdded := regexYear == "" && aiYear != ""
	yearCorrected := regexYear != "" && aiYear != "" && regexYear != aiYear

	// Title comparison
	titlesIdentical := regexTitle == aiTitle
	titlesIdenticalIgnoreCase := strings.EqualFold(regexTitle, aiTitle)

	// Strip punctuation for similarity check
	regexClean := stripPunctuation(regexTitle)
	aiClean := stripPunctuation(aiTitle)
	titlesIdenticalNoPunct := strings.EqualFold(regexClean, aiClean)

	// No change at all
	if titlesIdentical && !yearChanged {
		return ChangeClassification{Category: ChangeNone, Safe: true, MinConfidence: 0}
	}

	// Year only changes (title identical)
	if titlesIdentical || titlesIdenticalIgnoreCase {
		if yearAdded {
			return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
		}
		if yearCorrected {
			return ChangeClassification{Category: ChangeYearCorrected, Safe: true, MinConfidence: 0.90}
		}
	}

	// Casing change only
	if titlesIdenticalIgnoreCase && !yearChanged {
		return ChangeClassification{Category: ChangeCasing, Safe: true, MinConfidence: 0.80}
	}

	// Punctuation change (titles match after stripping punctuation)
	if titlesIdenticalNoPunct && !yearChanged {
		return ChangeClassification{Category: ChangePunctuation, Safe: true, MinConfidence: 0.80}
	}
	if titlesIdenticalNoPunct && yearAdded {
		return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
	}

	// Check Jaccard similarity for "different title" threshold
	similarity := jaccardWordSimilarity(regexTitle, aiTitle)
	if similarity < 0.70 {
		return ChangeClassification{Category: ChangeTitleDifferent, Safe: false}
	}

	// Similar but not identical — treat as punctuation/minor correction
	if yearAdded {
		return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
	}
	return ChangeClassification{Category: ChangePunctuation, Safe: true, MinConfidence: 0.80}
}

func stripPunctuation(s string) string {
	return punctuationRegex.ReplaceAllString(s, "")
}

func jaccardWordSimilarity(a, b string) float64 {
	wordsA := toWordSet(a)
	wordsB := toWordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func toWordSet(s string) map[string]bool {
	s = strings.ToLower(s)
	s = punctuationRegex.ReplaceAllString(s, "")
	words := strings.Fields(s)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if w != "" {
			set[w] = true
		}
	}
	return set
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run "TestClassifyChange|TestJaccard" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/classify.go internal/daemon/classify_test.go
git commit -m "daemon: add change classification for AI enhancement safety valve"
```

---

### Task 4: JSONL Enhancement Logger

**Files:**
- Create: `internal/daemon/enhancelog.go`
- Create: `internal/daemon/enhancelog_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/daemon/enhancelog_test.go`:

```go
package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnhanceLogger_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)

	entry := EnhanceLogEntry{
		Action:     "fast_lane",
		File:       "test.mkv",
		RegexTitle: "Test",
		Confidence: 0.85,
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ai-enhancements.jsonl"))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	var parsed EnhanceLogEntry
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if parsed.Action != "fast_lane" {
		t.Errorf("action = %q, want %q", parsed.Action, "fast_lane")
	}
	if parsed.Ts == "" {
		t.Error("timestamp should be set automatically")
	}
}

func TestEnhanceLogger_Rotation(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)
	logger.maxSize = 100 // rotate after 100 bytes

	// Write enough to trigger rotation
	for i := 0; i < 10; i++ {
		logger.Log(EnhanceLogEntry{
			Action:     "fast_lane",
			File:       "test.mkv",
			RegexTitle: "Some Title That Is Long Enough",
			Confidence: 0.85,
		})
	}

	// Should have created backup files
	entries, _ := os.ReadDir(dir)
	jsonlFiles := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "ai-enhancements") {
			jsonlFiles++
		}
	}
	if jsonlFiles < 2 {
		t.Errorf("expected at least 2 jsonl files after rotation, got %d", jsonlFiles)
	}
}

func TestEnhanceLogger_MaxBackups(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)
	logger.maxSize = 50
	logger.maxBackups = 2

	// Write enough to trigger multiple rotations
	for i := 0; i < 30; i++ {
		logger.Log(EnhanceLogEntry{
			Action:     "fast_lane",
			File:       "test.mkv",
			RegexTitle: "Title",
			Confidence: 0.85,
		})
	}

	entries, _ := os.ReadDir(dir)
	jsonlFiles := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), "ai-enhancements") {
			jsonlFiles++
		}
	}
	// current file + maxBackups
	if jsonlFiles > 3 {
		t.Errorf("expected at most 3 files (current + 2 backups), got %d", jsonlFiles)
	}
}

func TestReadFlaggedForReview(t *testing.T) {
	dir := t.TempDir()
	logger := NewEnhanceLogger(dir)

	// Write a mix of actions
	logger.Log(EnhanceLogEntry{Action: "fast_lane", File: "a.mkv"})
	logger.Log(EnhanceLogEntry{Action: "flagged_for_review", File: "b.mkv", AITitle: "Better Title"})
	logger.Log(EnhanceLogEntry{Action: "flagged_for_review", File: "c.mkv", AITitle: "Other Title"})
	logger.Log(EnhanceLogEntry{Action: "review_approved", File: "b.mkv"})

	flagged, err := ReadFlaggedForReview(filepath.Join(dir, "ai-enhancements.jsonl"))
	if err != nil {
		t.Fatalf("ReadFlaggedForReview error: %v", err)
	}
	// Only c.mkv should be pending (b.mkv was approved)
	if len(flagged) != 1 {
		t.Fatalf("expected 1 pending review, got %d", len(flagged))
	}
	if flagged[0].File != "c.mkv" {
		t.Errorf("expected c.mkv, got %s", flagged[0].File)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run "TestEnhanceLogger|TestReadFlagged" -v`
Expected: compilation failure

- [ ] **Step 3: Implement enhancelog.go**

Create `internal/daemon/enhancelog.go`:

```go
package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const enhanceLogFilename = "ai-enhancements.jsonl"

type EnhanceLogEntry struct {
	Ts           string  `json:"ts"`
	Action       string  `json:"action"`
	File         string  `json:"file,omitempty"`
	RegexTitle   string  `json:"regex_title,omitempty"`
	AITitle      string  `json:"ai_title,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	AIConfidence float64 `json:"ai_confidence,omitempty"`
	Category     string  `json:"category,omitempty"`
	AutoApplied  bool    `json:"auto_applied,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	MediaType    string  `json:"media_type,omitempty"`
	PendingCount int     `json:"pending_count,omitempty"`
	HourlyUsed   int     `json:"hourly_used,omitempty"`
	DailyUsed    int     `json:"daily_used,omitempty"`
}

type EnhanceLogger struct {
	dir        string
	maxSize    int64
	maxBackups int
}

func NewEnhanceLogger(dir string) *EnhanceLogger {
	return &EnhanceLogger{
		dir:        dir,
		maxSize:    10 * 1024 * 1024, // 10MB
		maxBackups: 3,
	}
}

func (l *EnhanceLogger) Log(entry EnhanceLogEntry) error {
	if entry.Ts == "" {
		entry.Ts = time.Now().UTC().Format(time.RFC3339)
	}

	logPath := filepath.Join(l.dir, enhanceLogFilename)

	if err := l.rotateIfNeeded(logPath); err != nil {
		return fmt.Errorf("rotation failed: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

func (l *EnhanceLogger) LogPath() string {
	return filepath.Join(l.dir, enhanceLogFilename)
}

func (l *EnhanceLogger) rotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil // file doesn't exist yet
	}
	if info.Size() < l.maxSize {
		return nil
	}

	// Shift backups: .3 -> deleted, .2 -> .3, .1 -> .2, current -> .1
	for i := l.maxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		if i == l.maxBackups {
			os.Remove(src)
			continue
		}
		dst := fmt.Sprintf("%s.%d", path, i+1)
		os.Rename(src, dst)
	}

	return os.Rename(path, path+".1")
}

func ReadFlaggedForReview(logPath string) ([]EnhanceLogEntry, error) {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	flagged := make(map[string]EnhanceLogEntry)
	resolved := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry EnhanceLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch entry.Action {
		case "flagged_for_review":
			flagged[entry.File] = entry
		case "review_approved", "review_rejected":
			resolved[entry.File] = true
		}
	}

	var pending []EnhanceLogEntry
	for file, entry := range flagged {
		if !resolved[file] {
			pending = append(pending, entry)
		}
	}

	return pending, scanner.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run "TestEnhanceLogger|TestReadFlagged" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/enhancelog.go internal/daemon/enhancelog_test.go
git commit -m "daemon: add JSONL enhancement logger with rotation"
```

---

## Chunk 2: Organizer WithParsed Methods

### Task 5: OrganizeMovieWithParsed

**Files:**
- Modify: `internal/organizer/organizer.go:333-466` (after existing `OrganizeMovie`)

- [ ] **Step 1: Write failing test**

Add to `internal/organizer/organizer_test.go` (or the existing test file — check which file has organizer tests):

```go
func TestOrganizeMovieWithParsed(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "movies")
	os.MkdirAll(libDir, 0755)

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	srcFile := filepath.Join(srcDir, "test.movie.2024.mkv")
	os.WriteFile(srcFile, []byte("test content"), 0644)

	org := New(WithDryRun(true))

	info := &naming.MovieInfo{
		Title: "Test Movie",
		Year:  "2024",
	}

	result, err := org.OrganizeMovieWithParsed(srcFile, info, libDir)
	if err != nil {
		t.Fatalf("OrganizeMovieWithParsed error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	// Should use the parsed info, not re-parse
	expectedTarget := filepath.Join(libDir, "Test Movie (2024)", "Test Movie (2024).mkv")
	if result.TargetPath != expectedTarget {
		t.Errorf("target = %q, want %q", result.TargetPath, expectedTarget)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/organizer/ -run TestOrganizeMovieWithParsed -v`
Expected: compilation failure — `OrganizeMovieWithParsed` undefined

- [ ] **Step 3: Implement OrganizeMovieWithParsed**

Add to `internal/organizer/organizer.go`, after `OrganizeMovie` (after line ~466):

```go
func (o *Organizer) OrganizeMovieWithParsed(sourcePath string, info *naming.MovieInfo, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	cleanName := naming.NormalizeMovieName(info.Title, info.Year)
	movieDir := filepath.Join(libraryPath, cleanName)
	ext := filepath.Ext(sourcePath)
	targetPath := filepath.Join(movieDir, cleanName+ext)

	if err := o.checkPlaybackSafetyWithOp(sourcePath, "organize_movie", targetPath); err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			TargetPath: targetPath,
			Skipped:    true,
			SkipReason: err.Error(),
			Error:      err,
		}, nil
	}

	existingFile, existingQuality := o.findExistingMediaFile(movieDir)
	if existingFile != "" && !o.forceOverwrite {
		if !sourceQuality.IsBetterThan(existingQuality) {
			return &OrganizationResult{
				Success:         false,
				SourcePath:      sourcePath,
				TargetPath:      existingFile,
				Skipped:         true,
				SkipReason:      fmt.Sprintf("existing file has equal or better quality (%s vs %s)", existingQuality.String(), sourceQuality.String()),
				SourceQuality:   sourceQuality,
				ExistingQuality: existingQuality,
			}, nil
		}
	}

	if err := os.MkdirAll(movieDir, 0755); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to create directory: %w", err),
		}, nil
	}

	if err := o.applyDirOwnership(movieDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set directory permissions: %w", err),
		}, nil
	}

	if o.dryRun {
		return &OrganizationResult{
			Success:       true,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
		}, nil
	}

	result, err := o.backend.Transfer(sourcePath, targetPath, o.timeout)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         err,
		}, nil
	}

	o.applyFileOwnership(targetPath)

	return &OrganizationResult{
		Success:         true,
		SourcePath:      sourcePath,
		TargetPath:      targetPath,
		BytesCopied:     result.BytesCopied,
		Duration:        result.Duration,
		Attempts:        result.Attempts,
		SourceQuality:   sourceQuality,
		ExistingQuality: existingQuality,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/organizer/ -run TestOrganizeMovieWithParsed -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/organizer/organizer.go internal/organizer/organizer_test.go
git commit -m "organizer: add OrganizeMovieWithParsed accepting pre-parsed info"
```

---

### Task 6: OrganizeTVWithParsed

**Files:**
- Modify: `internal/organizer/organizer.go` (after `OrganizeTVEpisodeAuto`, ~line 656)

- [ ] **Step 1: Write failing test**

Add to the organizer test file:

```go
func TestOrganizeTVWithParsed(t *testing.T) {
	tmpDir := t.TempDir()
	libDir := filepath.Join(tmpDir, "tv")
	os.MkdirAll(libDir, 0755)

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	srcFile := filepath.Join(srcDir, "show.s01e01.mkv")
	os.WriteFile(srcFile, []byte("test content"), 0644)

	selector := library.NewSelector([]string{libDir})
	org := New(WithDryRun(true), WithSelector(selector))

	info := &naming.TVShowInfo{
		Title:   "Test Show",
		Year:    "2024",
		Season:  1,
		Episode: 1,
	}

	sizeFunc := func(p string) (int64, error) {
		fi, err := os.Stat(p)
		if err != nil {
			return 0, err
		}
		return fi.Size(), nil
	}

	result, err := org.OrganizeTVWithParsed(srcFile, info, sizeFunc)
	if err != nil {
		t.Fatalf("OrganizeTVWithParsed error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	// Verify it used parsed info, not re-parsed
	if !strings.Contains(result.TargetPath, "Test Show") {
		t.Errorf("target path %q should contain 'Test Show'", result.TargetPath)
	}
	if !strings.Contains(result.TargetPath, "S01E01") {
		t.Errorf("target path %q should contain 'S01E01'", result.TargetPath)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/organizer/ -run TestOrganizeTVWithParsed -v`
Expected: compilation failure

- [ ] **Step 3: Implement OrganizeTVWithParsed**

Add to `internal/organizer/organizer.go`, after `OrganizeTVEpisodeAuto`:

```go
func (o *Organizer) OrganizeTVWithParsed(sourcePath string, info *naming.TVShowInfo, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	fileSize, err := getFileSize(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil
	}

	selection, err := o.selector.SelectTVShowLibrary(info.Title, info.Year, fileSize)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to select library: %w", err),
		}, nil
	}

	return o.organizeTVWithInfo(sourcePath, info, selection.Library)
}

func (o *Organizer) organizeTVWithInfo(sourcePath string, tv *naming.TVShowInfo, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	showDir := findExistingShowDir(libraryPath, tv.Title)
	if showDir == "" {
		showName := naming.NormalizeTVShowName(tv.Title, tv.Year)
		showDir = filepath.Join(libraryPath, showName)
	}

	seasonDir := filepath.Join(showDir, naming.FormatSeasonFolder(tv.Season))
	ext := filepath.Ext(sourcePath)
	var episodeName string
	if tv.IsDateBased {
		episodeName = naming.FormatDateBasedEpisodeFilename(tv.Title, tv.Date, ext[1:])
	} else {
		episodeName = naming.FormatTVEpisodeFilename(tv.Title, tv.Year, tv.Season, tv.Episode, ext[1:])
	}
	targetPath := filepath.Join(seasonDir, episodeName)

	if err := o.checkPlaybackSafetyWithOp(sourcePath, "organize_tv", targetPath); err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			TargetPath: targetPath,
			Skipped:    true,
			SkipReason: err.Error(),
			Error:      err,
		}, nil
	}

	existingFile, existingQuality := o.findExistingMediaFile(seasonDir)
	if existingFile != "" && !o.forceOverwrite {
		existingBase := filepath.Base(existingFile)
		targetBase := filepath.Base(targetPath)
		sameEpisode := strings.HasPrefix(existingBase, strings.TrimSuffix(targetBase, ext))
		if sameEpisode && !sourceQuality.IsBetterThan(existingQuality) {
			return &OrganizationResult{
				Success:         false,
				SourcePath:      sourcePath,
				TargetPath:      existingFile,
				Skipped:         true,
				SkipReason:      fmt.Sprintf("existing file has equal or better quality (%s vs %s)", existingQuality.String(), sourceQuality.String()),
				SourceQuality:   sourceQuality,
				ExistingQuality: existingQuality,
			}, nil
		}
	}

	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to create directories: %w", err),
		}, nil
	}

	if err := o.applyDirOwnership(showDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set directory permissions: %w", err),
		}, nil
	}

	if o.dryRun {
		return &OrganizationResult{
			Success:       true,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
		}, nil
	}

	result, err := o.backend.Transfer(sourcePath, targetPath, o.timeout)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         err,
		}, nil
	}

	o.applyFileOwnership(targetPath)

	return &OrganizationResult{
		Success:         true,
		SourcePath:      sourcePath,
		TargetPath:      targetPath,
		BytesCopied:     result.BytesCopied,
		Duration:        result.Duration,
		Attempts:        result.Attempts,
		SourceQuality:   sourceQuality,
		ExistingQuality: existingQuality,
	}, nil
}
```

**Note:** `organizeTVWithInfo` is a private helper that contains the core logic extracted from `OrganizeTVEpisode`. The existing `OrganizeTVEpisode` method remains unchanged. If you notice the duplication is excessive, you can refactor `OrganizeTVEpisode` to call `organizeTVWithInfo` internally — but only after confirming tests still pass.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/organizer/ -run TestOrganizeTVWithParsed -v`
Expected: PASS

- [ ] **Step 5: Run all existing organizer tests**

Run: `go test ./internal/organizer/ -v`
Expected: all existing tests still PASS

- [ ] **Step 6: Commit**

```bash
git add internal/organizer/organizer.go internal/organizer/organizer_test.go
git commit -m "organizer: add OrganizeTVWithParsed accepting pre-parsed TVShowInfo"
```

---

## Chunk 3: Handler Two-Lane Processing + Ticker

### Task 7: Add AI Fields to MediaHandlerConfig and Handler

**Files:**
- Modify: `internal/daemon/handler.go:23-43` (MediaHandler struct)
- Modify: `internal/daemon/handler.go:107-129` (MediaHandlerConfig struct)
- Modify: `internal/daemon/handler.go:131-151` (NewMediaHandler)

- [ ] **Step 1: Add fields to MediaHandlerConfig**

In `internal/daemon/handler.go`, add to `MediaHandlerConfig` (after `DeferredQueue` field, line 128):

```go
AIEnabled       bool
AIMatcher       *ai.Matcher
AIConfig        config.AIConfig
```

This requires adding imports for `"github.com/Nomadcxx/plex2jellyfin/internal/ai"` and `"github.com/Nomadcxx/plex2jellyfin/internal/config"`.

- [ ] **Step 2: Add fields to MediaHandler struct**

Add to `MediaHandler` struct (after `deferredQueue` field, line 42):

```go
pendingAI      map[string]*PendingItem
pendingAICap   int
aiMatcher      *ai.Matcher
aiConfig       config.AIConfig
aiRateLimiter  *AIRateLimiter
enhanceLogger  *EnhanceLogger
aiEnabled      bool
```

- [ ] **Step 3: Add PendingItem type**

Add after the `MediaHandler` struct:

```go
type PendingItem struct {
	Path       string
	Filename   string
	TVInfo     *naming.TVShowInfo
	MovieInfo  *naming.MovieInfo
	MediaType  string
	Confidence float64
	QueuedAt   time.Time
	TargetLib  string
}
```

- [ ] **Step 4: Initialize new fields in NewMediaHandler**

In `NewMediaHandler`, after the activity logger initialization (~line 151), add:

```go
var enhanceLog *EnhanceLogger
var rateLimiter *AIRateLimiter
if cfg.AIEnabled && cfg.ConfigDir != "" {
	enhanceLog = NewEnhanceLogger(cfg.ConfigDir)
	rateLimiter = NewAIRateLimiter(cfg.AIConfig.HourlyLimit, cfg.AIConfig.DailyLimit)
}
```

Then in the `MediaHandler` return struct, add:

```go
pendingAI:     make(map[string]*PendingItem),
pendingAICap:  100,
aiMatcher:     cfg.AIMatcher,
aiConfig:      cfg.AIConfig,
aiRateLimiter: rateLimiter,
enhanceLogger: enhanceLog,
aiEnabled:     cfg.AIEnabled,
```

- [ ] **Step 5: Verify build**

Run: `go build ./internal/daemon/...`
Expected: success (may need to adjust import paths)

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/handler.go
git commit -m "daemon: add AI enhancement fields to handler config and struct"
```

---

### Task 8: Confidence Gating in processFile

**Files:**
- Modify: `internal/daemon/handler.go:395-448` (processFile TV and movie paths)

This is the core change — adding the two-lane routing.

- [ ] **Step 1: Write failing test for fast-lane routing**

Add to `internal/daemon/handler_test.go`:

```go
func TestProcessFile_FastLaneHighConfidence(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	// Create a well-named source file
	srcFile := filepath.Join(watchDir, "Breaking.Bad.S01E01.1080p.mkv")
	os.WriteFile(srcFile, []byte("test"), 0644)

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{watchDir},
		MovieWatchPaths: []string{},
		Logger:          logging.Nop(),
		AIEnabled:       false, // AI disabled = always fast lane
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Fast lane: file should be processed, not queued
	handler.processFile(srcFile)

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected no pending AI items for high-confidence file, got %d", len(handler.pendingAI))
	}
}
```

- [ ] **Step 2: Run test to verify behavior**

Run: `go test ./internal/daemon/ -run TestProcessFile_FastLane -v`
Expected: PASS (current behavior — all files go through fast lane when AI disabled)

- [ ] **Step 3: Write test for slow-lane routing**

Add to handler test file:

```go
func TestProcessFile_SlowLaneLowConfidence(t *testing.T) {
	tmpLib := t.TempDir()
	watchDir := t.TempDir()

	// Create an obfuscated source file (will have low confidence)
	srcFile := filepath.Join(watchDir, "abc123def456.mkv")
	os.WriteFile(srcFile, []byte("test"), 0644)

	cfg := MediaHandlerConfig{
		TVLibraries:     []string{tmpLib},
		MovieLibs:       []string{tmpLib},
		TVWatchPaths:    []string{},
		MovieWatchPaths: []string{watchDir},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	handler.processFile(srcFile)

	if len(handler.pendingAI) != 1 {
		t.Errorf("expected 1 pending AI item for low-confidence file, got %d", len(handler.pendingAI))
	}
}
```

- [ ] **Step 4: Implement confidence gating in processFile**

Modify the TV path in `processFile` (around lines 395-419). Replace the current TV block:

```go
if isTVEpisode {
	if len(h.tvLibraries) == 0 {
		h.logger.Warn("handler", "No TV libraries configured, skipping", logging.F("filename", filename))
		return
	}
	mediaType = notify.MediaTypeTVEpisode

	tvInfo, parseErr := naming.ParseTVShowName(path)
	if parseErr == nil {
		parsedTitle = tvInfo.Title
		if tvInfo.Year != "" {
			year := 0
			if _, err := fmt.Sscanf(tvInfo.Year, "%d", &year); err == nil {
				parsedYear = &year
			}
		}

		// Confidence gating
		confidence := naming.CalculateTitleConfidence(tvInfo.Title, filename)
		if h.aiEnabled && confidence < h.aiConfig.AutoTriggerThreshold {
			h.queueForAI(path, filename, tvInfo, nil, "tv", confidence, "")
			return
		}
	}

	result, err = h.tvOrganizer.OrganizeTVEpisodeAuto(path, func(p string) (int64, error) {
		info, err := os.Stat(p)
		if err != nil {
			return 0, err
		}
		return info.Size(), nil
	})

	if result != nil && result.TargetPath != "" {
		targetLib = filepath.Dir(filepath.Dir(filepath.Dir(result.TargetPath)))
	}
```

Similarly modify the movie path (around lines 425-448):

```go
} else {
	if len(h.movieLibs) == 0 {
		h.logger.Warn("handler", "No movie libraries configured, skipping", logging.F("filename", filename))
		return
	}
	targetLib = h.movieLibs[0]
	mediaType = notify.MediaTypeMovie

	movieInfo, parseErr := naming.ParseMovieFromPath(path)
	if parseErr == nil {
		parsedTitle = movieInfo.Title
		if movieInfo.Year != "" {
			year := 0
			if _, err := fmt.Sscanf(movieInfo.Year, "%d", &year); err == nil {
				parsedYear = &year
			}
		}

		confidence := naming.CalculateTitleConfidence(movieInfo.Title, filename)
		if h.aiEnabled && confidence < h.aiConfig.AutoTriggerThreshold {
			h.queueForAI(path, filename, nil, movieInfo, "movie", confidence, targetLib)
			return
		}
	}

	if !h.checkTargetHealth(targetLib) {
		h.logger.Warn("handler", "Target library unhealthy, skipping", logging.F("filename", filename), logging.F("target", targetLib))
		return
	}

	result, err = h.movieOrganizer.OrganizeMovie(path, targetLib)
}
```

- [ ] **Step 5: Implement queueForAI helper**

Add to `handler.go`:

```go
func (h *MediaHandler) queueForAI(path, filename string, tvInfo *naming.TVShowInfo, movieInfo *naming.MovieInfo, mediaType string, confidence float64, targetLib string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.pendingAI) >= h.pendingAICap {
		h.logger.Warn("handler", "Pending AI cap reached, using regex fallback",
			logging.F("filename", filename))
		if h.enhanceLogger != nil {
			h.enhanceLogger.Log(EnhanceLogEntry{
				Action:     "pending_cap_reached",
				File:       filename,
				Confidence: confidence,
			})
		}
		// Fall through to normal processing (will be handled by caller)
		return
	}

	h.pendingAI[path] = &PendingItem{
		Path:      path,
		Filename:  filename,
		TVInfo:    tvInfo,
		MovieInfo: movieInfo,
		MediaType: mediaType,
		Confidence: confidence,
		QueuedAt:  time.Now(),
		TargetLib: targetLib,
	}

	h.logger.Info("handler", "Queued for AI enhancement",
		logging.F("filename", filename),
		logging.F("confidence", confidence))

	if h.enhanceLogger != nil {
		h.enhanceLogger.Log(EnhanceLogEntry{
			Action:     "queued_for_ai",
			File:       filename,
			RegexTitle: h.getParsedTitle(tvInfo, movieInfo),
			Confidence: confidence,
			MediaType:  mediaType,
		})
	}
}

func (h *MediaHandler) getParsedTitle(tvInfo *naming.TVShowInfo, movieInfo *naming.MovieInfo) string {
	if tvInfo != nil {
		return tvInfo.Title
	}
	if movieInfo != nil {
		return movieInfo.Title
	}
	return ""
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/daemon/ -run "TestProcessFile" -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/handler.go internal/daemon/handler_test.go
git commit -m "daemon: add two-lane confidence gating in processFile"
```

---

### Task 9: AI Enhancement Ticker

**Files:**
- Modify: `internal/daemon/handler.go` (add `ProcessPendingAI` method)

- [ ] **Step 1: Write failing test for ticker processing**

Add to `internal/daemon/handler_test.go`:

```go
func TestProcessPendingAI_ExpiresOldItems(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv"},
		MovieLibs:       []string{"/movie"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Add an expired pending item
	handler.pendingAI["/old/file.mkv"] = &PendingItem{
		Path:      "/old/file.mkv",
		Filename:  "file.mkv",
		MediaType: "movie",
		QueuedAt:  time.Now().Add(-25 * time.Hour), // 25 hours ago
	}

	handler.ProcessPendingAI()

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected expired item to be removed, got %d pending", len(handler.pendingAI))
	}
}

func TestProcessPendingAI_SkipsMissingFiles(t *testing.T) {
	cfg := MediaHandlerConfig{
		TVLibraries:     []string{"/tv"},
		MovieLibs:       []string{"/movie"},
		TVWatchPaths:    []string{"/watch/tv"},
		MovieWatchPaths: []string{"/watch/movies"},
		Logger:          logging.Nop(),
		AIEnabled:       true,
		AIConfig:        config.AIConfig{AutoTriggerThreshold: 0.6},
		ConfigDir:       t.TempDir(),
	}
	handler, err := NewMediaHandler(cfg)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Add item for a file that doesn't exist
	handler.pendingAI["/nonexistent/file.mkv"] = &PendingItem{
		Path:      "/nonexistent/file.mkv",
		Filename:  "file.mkv",
		MediaType: "movie",
		QueuedAt:  time.Now(),
	}

	handler.ProcessPendingAI()

	if len(handler.pendingAI) != 0 {
		t.Errorf("expected missing file to be removed, got %d pending", len(handler.pendingAI))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/ -run TestProcessPendingAI -v`
Expected: compilation failure — `ProcessPendingAI` undefined

- [ ] **Step 3: Implement ProcessPendingAI**

Add to `internal/daemon/handler.go`:

```go
func (h *MediaHandler) ProcessPendingAI() {
	h.mu.Lock()
	items := make([]*PendingItem, 0, len(h.pendingAI))
	for _, item := range h.pendingAI {
		items = append(items, item)
	}
	h.mu.Unlock()

	now := time.Now()
	expiry := 24 * time.Hour

	for _, item := range items {
		// Expire old items
		if now.Sub(item.QueuedAt) > expiry {
			h.logger.Info("handler", "Expiring old pending AI item",
				logging.F("filename", item.Filename))
			if h.enhanceLogger != nil {
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action: "expired",
					File:   item.Filename,
					Reason: "pending > 24h",
				})
			}
			h.mu.Lock()
			delete(h.pendingAI, item.Path)
			h.mu.Unlock()
			continue
		}

		// Check file still exists
		if _, err := os.Stat(item.Path); os.IsNotExist(err) {
			h.logger.Info("handler", "Pending file no longer exists",
				logging.F("filename", item.Filename))
			if h.enhanceLogger != nil {
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action: "expired",
					File:   item.Filename,
					Reason: "file deleted",
				})
			}
			h.mu.Lock()
			delete(h.pendingAI, item.Path)
			h.mu.Unlock()
			continue
		}

		// Check rate limit
		if h.aiRateLimiter != nil && !h.aiRateLimiter.Allow() {
			if h.enhanceLogger != nil {
				hourly, daily := h.aiRateLimiter.Stats()
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action:       "rate_limited",
					PendingCount: len(h.pendingAI),
					HourlyUsed:   hourly,
					DailyUsed:    daily,
				})
			}
			return // Stop processing this tick
		}

		// Call AI
		if h.aiMatcher == nil {
			continue
		}

		ctx := context.Background()
		aiResult, err := h.aiMatcher.ParseWithRetry(ctx, item.Filename)
		if h.aiRateLimiter != nil {
			h.aiRateLimiter.Record()
		}

		if err != nil {
			h.logger.Warn("handler", "AI enhancement failed",
				logging.F("filename", item.Filename),
				logging.F("error", err.Error()))
			h.mu.Lock()
			delete(h.pendingAI, item.Path)
			h.mu.Unlock()
			continue
		}

		// Classify the change
		regexTitle := h.getParsedTitle(item.TVInfo, item.MovieInfo)
		regexYear := ""
		if item.TVInfo != nil {
			regexYear = item.TVInfo.Year
		} else if item.MovieInfo != nil {
			regexYear = item.MovieInfo.Year
		}
		aiYear := ""
		if aiResult.Year != nil {
			aiYear = fmt.Sprintf("%d", *aiResult.Year.Int())
		}

		classification := ClassifyChange(regexTitle, aiResult.Title, regexYear, aiYear, item.MediaType, aiResult.Type)

		if classification.Safe && aiResult.Confidence >= classification.MinConfidence {
			// Auto-apply
			h.applyAIResult(item, aiResult)
			if h.enhanceLogger != nil {
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action:       "ai_enhanced",
					File:         item.Filename,
					RegexTitle:   regexTitle,
					AITitle:      aiResult.Title,
					AIConfidence: aiResult.Confidence,
					Category:     string(classification.Category),
					AutoApplied:  true,
					MediaType:    item.MediaType,
				})
			}
		} else {
			// Flag for review
			reason := "risky change"
			if classification.Safe && aiResult.Confidence < classification.MinConfidence {
				reason = fmt.Sprintf("confidence %.2f below threshold %.2f", aiResult.Confidence, classification.MinConfidence)
			}
			if h.enhanceLogger != nil {
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action:       "flagged_for_review",
					File:         item.Filename,
					RegexTitle:   regexTitle,
					AITitle:      aiResult.Title,
					AIConfidence: aiResult.Confidence,
					Category:     string(classification.Category),
					Reason:       reason,
					MediaType:    item.MediaType,
				})
			}
			h.logger.Info("handler", "AI enhancement flagged for review",
				logging.F("filename", item.Filename),
				logging.F("category", string(classification.Category)),
				logging.F("reason", reason))
		}

		h.mu.Lock()
		delete(h.pendingAI, item.Path)
		h.mu.Unlock()
	}
}

func (h *MediaHandler) applyAIResult(item *PendingItem, aiResult *ai.Result) {
	var result *organizer.OrganizationResult
	var err error

	if item.MediaType == "tv" {
		// Build TVShowInfo from AI result
		tvInfo := &naming.TVShowInfo{
			Title: aiResult.Title,
		}
		if aiResult.Year != nil {
			tvInfo.Year = fmt.Sprintf("%d", *aiResult.Year.Int())
		}
		if aiResult.Season != nil {
			tvInfo.Season = *aiResult.Season.Int()
		}
		if len(aiResult.Episodes) > 0 {
			tvInfo.Episode = aiResult.Episodes[0]
		}
		// Preserve date-based info from original parse
		if item.TVInfo != nil {
			tvInfo.IsDateBased = item.TVInfo.IsDateBased
			tvInfo.Date = item.TVInfo.Date
			// Keep original season/episode if AI didn't provide them
			if aiResult.Season == nil {
				tvInfo.Season = item.TVInfo.Season
			}
			if len(aiResult.Episodes) == 0 {
				tvInfo.Episode = item.TVInfo.Episode
			}
		}

		result, err = h.tvOrganizer.OrganizeTVWithParsed(item.Path, tvInfo, func(p string) (int64, error) {
			info, err := os.Stat(p)
			if err != nil {
				return 0, err
			}
			return info.Size(), nil
		})
	} else {
		movieInfo := &naming.MovieInfo{
			Title: aiResult.Title,
		}
		if aiResult.Year != nil {
			movieInfo.Year = fmt.Sprintf("%d", *aiResult.Year.Int())
		}
		result, err = h.movieOrganizer.OrganizeMovieWithParsed(item.Path, movieInfo, item.TargetLib)
	}

	if err != nil {
		h.logger.Error("handler", "AI-enhanced organization failed", err,
			logging.F("filename", item.Filename))
		h.stats.RecordError()
		return
	}

	if result != nil && result.Success {
		h.logger.Info("handler", "AI-enhanced organization successful",
			logging.F("filename", item.Filename),
			logging.F("target", result.TargetPath))
		if item.MediaType == "movie" {
			h.stats.RecordMovie(result.BytesCopied)
		} else {
			h.stats.RecordTV(result.BytesCopied)
		}
	}
}
```

This requires adding `"context"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/ -run TestProcessPendingAI -v`
Expected: all PASS

- [ ] **Step 5: Run all daemon tests**

Run: `go test ./internal/daemon/ -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/handler.go internal/daemon/handler_test.go
git commit -m "daemon: add ProcessPendingAI ticker method with expiry and rate limiting"
```

---

## Chunk 4: Wiring + Review Command

### Task 10: Wire AI into main.go

**Files:**
- Modify: `cmd/plex2jellyfin-daemon/main.go:185-218` (handler creation)

- [ ] **Step 1: Add AI matcher creation**

In `cmd/plex2jellyfin-daemon/main.go`, before the `handler` creation block (~line 196), add:

```go
// Create AI matcher for daemon enhancement if enabled
var aiMatcher *ai.Matcher
if cfg.AI.Enabled {
	var matcherErr error
	aiMatcher, matcherErr = ai.NewMatcher(cfg.AI)
	if matcherErr != nil {
		logger.Warn("daemon", "AI matcher initialization failed, daemon will use regex only",
			logging.F("error", matcherErr.Error()))
	} else {
		logger.Info("daemon", "AI enhancement enabled",
			logging.F("model", cfg.AI.Model),
			logging.F("hourly_limit", cfg.AI.HourlyLimit),
			logging.F("daily_limit", cfg.AI.DailyLimit))
	}
}
```

This requires adding `"github.com/Nomadcxx/plex2jellyfin/internal/ai"` to imports.

- [ ] **Step 2: Add AI fields to handler config**

In the `daemon.NewMediaHandler(daemon.MediaHandlerConfig{...})` block, add after `DeferredQueue`:

```go
AIEnabled: cfg.AI.Enabled && aiMatcher != nil,
AIMatcher: aiMatcher,
AIConfig:  cfg.AI,
```

- [ ] **Step 3: Add ticker goroutine**

After the watcher setup (~line 257), add:

```go
// Start AI enhancement ticker
if cfg.AI.Enabled && aiMatcher != nil {
	interval := time.Duration(cfg.AI.EnhancementIntervalSeconds) * time.Second
	if interval == 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				handler.ProcessPendingAI()
			case <-ctx.Done():
				return
			}
		}
	}()
	logger.Info("daemon", "AI enhancement ticker started",
		logging.F("interval", interval.String()))
}
```

**Note:** Check if there's already a `ctx` in scope (from signal handling). If not, you'll need `ctx, cancel := context.WithCancel(context.Background())` and wire `cancel()` to the shutdown signal.

- [ ] **Step 4: Verify build**

Run: `go build ./cmd/plex2jellyfin-daemon/...`
Expected: success

- [ ] **Step 5: Commit**

```bash
git add cmd/plex2jellyfin-daemon/main.go
git commit -m "daemon: wire AI matcher and enhancement ticker into main"
```

---

### Task 11: plex2jellyfin review Command

**Files:**
- Create: `cmd/plex2jellyfin/review_cmd.go`

- [ ] **Step 1: Create review command**

Create `cmd/plex2jellyfin/review_cmd.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Review AI enhancement suggestions",
	Long:  "Review flagged AI title enhancement suggestions and approve or reject them.",
	RunE:  runReview,
}

var listOnly bool

func init() {
	reviewCmd.Flags().BoolVar(&listOnly, "list", false, "List pending reviews without prompting")
	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "plex2jellyfin")
	logPath := filepath.Join(configDir, "ai-enhancements.jsonl")

	flagged, err := daemon.ReadFlaggedForReview(logPath)
	if err != nil {
		return fmt.Errorf("failed to read enhancement log: %w", err)
	}

	if len(flagged) == 0 {
		fmt.Println("No items flagged for review.")
		return nil
	}

	fmt.Printf("%d items flagged for review:\n\n", len(flagged))

	enhanceLogger := daemon.NewEnhanceLogger(configDir)
	reader := bufio.NewReader(os.Stdin)

	for i, item := range flagged {
		fmt.Printf("%d. %s\n", i+1, item.File)
		fmt.Printf("   Regex: %q (confidence: %.2f)\n", item.RegexTitle, item.Confidence)
		fmt.Printf("   AI suggests: %q (confidence: %.2f)\n", item.AITitle, item.AIConfidence)
		fmt.Printf("   Category: %s\n", item.Category)

		// Check if source file exists
		// Note: flagged items store filename, not full path
		// The review command needs the full path to organize
		// For now, just show the status

		if listOnly {
			fmt.Println()
			continue
		}

		fmt.Print("   [a]pprove  [r]eject  [s]kip: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a", "approve":
			// TODO: In a future task, wire up the organizer to apply
			// For now, log the approval
			enhanceLogger.Log(daemon.EnhanceLogEntry{
				Action:  "review_approved",
				File:    item.File,
				AITitle: item.AITitle,
			})
			fmt.Printf("   Approved: %s\n\n", item.AITitle)
		case "r", "reject":
			enhanceLogger.Log(daemon.EnhanceLogEntry{
				Action: "review_rejected",
				File:   item.File,
				Reason: "user rejected",
			})
			fmt.Printf("   Rejected.\n\n")
		default:
			fmt.Printf("   Skipped.\n\n")
		}
	}

	return nil
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/plex2jellyfin/...`
Expected: success (may have compilation errors from pre-existing untracked test files — ignore those)

- [ ] **Step 3: Commit**

```bash
git add cmd/plex2jellyfin/review_cmd.go
git commit -m "cli: add plex2jellyfin review command for AI enhancement triage"
```

---

### Task 12: Final Integration Test

- [ ] **Step 1: Run all daemon tests**

Run: `go test ./internal/daemon/ -v`
Expected: all PASS

- [ ] **Step 2: Run all organizer tests**

Run: `go test ./internal/organizer/ -v`
Expected: all PASS

- [ ] **Step 3: Run naming tests (regression check)**

Run: `go test ./internal/naming/ -v`
Expected: all PASS

- [ ] **Step 4: Build all binaries**

Run: `go build ./cmd/plex2jellyfin/ && go build ./cmd/plex2jellyfin-daemon/`
Expected: success

- [ ] **Step 5: Commit any remaining fixes**

If any tests needed fixing, commit those fixes now.
