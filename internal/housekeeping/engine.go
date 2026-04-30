// Package housekeeping provides automated detection and remediation of
// library issues (cross-volume duplicates, polluted folder names, orphan
// source dirs, stuck sync_log rows, …).
//
// The engine is split into two halves so detection can run on a slow
// schedule (e.g., hourly) without bursting fix work:
//
//   - Detect: enumerate libraries, classify issues, enqueue tasks into
//     housekeeping_tasks (idempotent via dedup_key).
//   - Drain: bounded worker pool that pulls tasks one at a time and
//     executes them, with retry/backoff and a per-cycle cap.
//
// "Auto" task kinds (move_merge, no_year_merge, orphan_source, stuck_sync)
// are executed by the worker. "Flag-only" kinds (year_mismatch,
// polluted_name, subdir_mismatch) are surfaced in the WebUI for human
// approval and never executed automatically.
package housekeeping

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
)

// Config controls detector behaviour and worker concurrency.
type Config struct {
	TVLibraries        []string
	MovieLibraries     []string
	WatchDirs          []string
	MaxConcurrentTasks int           // bounded worker pool size
	MaxTasksPerCycle   int           // cap on enqueues per detect run
	TaskRetryMax       int           // attempts before a task is failed
	TaskPauseBetween   time.Duration // sleep between task executions
	StuckSyncAfter     time.Duration // mark sync_log running > this as stuck
	DryRun             bool
}

// DefaultConfig returns the conservative defaults discussed in the plan.
func DefaultConfig() Config {
	return Config{
		MaxConcurrentTasks: 2,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       3,
		TaskPauseBetween:   2 * time.Second,
		StuckSyncAfter:     24 * time.Hour,
	}
}

// Engine owns detection + drain for housekeeping work.
type Engine struct {
	cfg        Config
	db         *database.MediaDB
	logger     *logging.Logger
	transferer transfer.Transferer
}

// NewEngine constructs an Engine.
func NewEngine(cfg Config, db *database.MediaDB, logger *logging.Logger) *Engine {
	if cfg.MaxConcurrentTasks <= 0 {
		cfg.MaxConcurrentTasks = 2
	}
	if cfg.MaxTasksPerCycle <= 0 {
		cfg.MaxTasksPerCycle = 50
	}
	if cfg.TaskRetryMax <= 0 {
		cfg.TaskRetryMax = 3
	}
	if cfg.StuckSyncAfter <= 0 {
		cfg.StuckSyncAfter = 24 * time.Hour
	}
	tr, _ := transfer.New(transfer.BackendRsync)
	return &Engine{cfg: cfg, db: db, logger: logger, transferer: tr}
}

// DetectResult summarises one detect cycle.
type DetectResult struct {
	CrossVolumeDupes int
	NoYearMerges     int
	YearMismatches   int
	PollutedNames    int
	OrphanSources    int
	StuckSyncs       int
	Enqueued         int
	Skipped          int
}

// Detect scans the configured libraries and watch dirs, enqueueing any
// fixable issue into housekeeping_tasks. Returns a summary.
func (e *Engine) Detect(ctx context.Context) (*DetectResult, error) {
	res := &DetectResult{}

	tvFolders := e.scanLibraries(e.cfg.TVLibraries)
	movieFolders := e.scanLibraries(e.cfg.MovieLibraries)

	cap := e.cfg.MaxTasksPerCycle
	enqueue := func(kind string, payload map[string]any, prio int) {
		if res.Enqueued >= cap {
			res.Skipped++
			return
		}
		id, err := e.db.EnqueueHousekeepingTask("housekeeping.detect", kind, payload, prio)
		if err != nil {
			e.logf("error", "enqueue task failed kind=%s err=%v", kind, err)
			res.Skipped++
			return
		}
		if id > 0 {
			res.Enqueued++
		}
	}

	// 1+2+3+4: same-title grouping for TV and movies
	e.detectDuplicateGroups(ctx, tvFolders, "tv", res, enqueue)
	e.detectDuplicateGroups(ctx, movieFolders, "movie", res, enqueue)

	// 5: orphan source dirs in watch directories
	e.detectOrphanSources(ctx, res, enqueue)

	// 6: stuck sync_log rows
	e.detectStuckSync(ctx, res, enqueue)

	return res, nil
}

// folder represents a library subdirectory.
type folder struct {
	library string
	path    string // absolute
	name    string // basename
	title   string // normalized
	year    string
	hasYear bool
	polluted bool
}

func (e *Engine) scanLibraries(libs []string) []folder {
	var out []folder
	for _, lib := range libs {
		entries, err := os.ReadDir(lib)
		if err != nil {
			e.logf("warn", "read library failed lib=%s err=%v", lib, err)
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			name := ent.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			title, year, hasYear := splitFolderName(name)
			polluted := looksPolluted(name)
			out = append(out, folder{
				library:  lib,
				path:     filepath.Join(lib, name),
				name:     name,
				title:    naming.NormalizeName(title),
				year:     year,
				hasYear:  hasYear,
				polluted: polluted,
			})
		}
	}
	return out
}

// splitFolderName extracts "Title (YYYY)" → ("Title", "YYYY", true).
func splitFolderName(name string) (title, year string, hasYear bool) {
	s := name
	if i := strings.LastIndex(s, "("); i >= 0 {
		rest := s[i+1:]
		if j := strings.Index(rest, ")"); j == 4 && isAllDigits(rest[:4]) {
			return strings.TrimSpace(s[:i]), rest[:4], true
		}
	}
	return strings.TrimSpace(s), "", false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// looksPolluted flags release-tag / language-tag / lowercase folder names.
func looksPolluted(name string) bool {
	upper := strings.ToUpper(name)
	tokens := []string{
		" 1080P", " 2160P", " 720P", " 480P",
		" WEB-DL", " WEBRIP", " BLURAY", " BDRIP", " HDTV", " AMZN",
		" X264", " X265", " H.264", " H.265", " HEVC",
		" DDP5.1", " DD5.1", " AAC2.0", " ATMOS",
		" ITA ", " ENG ", " FRE ", " GER ", " SPA ",
	}
	for _, t := range tokens {
		if strings.Contains(upper, t) {
			return true
		}
	}
	return false
}

// detectDuplicateGroups groups folders by normalized title and enqueues
// merges/flags for groups with >1 entry.
func (e *Engine) detectDuplicateGroups(ctx context.Context, folders []folder, kind string, res *DetectResult, enqueue func(string, map[string]any, int)) {
	groups := map[string][]folder{}
	for _, f := range folders {
		if f.title == "" {
			continue
		}
		groups[f.title] = append(groups[f.title], f)
		if f.polluted {
			res.PollutedNames++
			enqueue(database.TaskKindPollutedName, map[string]any{
				"path":    f.path,
				"name":    f.name,
				"library": f.library,
				"kind":    kind,
			}, 200)
		}
	}

	for title, group := range groups {
		if len(group) < 2 {
			continue
		}
		if ctx.Err() != nil {
			return
		}

		// Pick canonical: prefer year-bearing, then largest by entry count
		// (proxy for "main" location). Real episode count would be more
		// accurate but a directory walk per group is too expensive here.
		canonical := chooseCanonical(group)

		// Bucket the rest by reason.
		yearMismatch := false
		for _, f := range group {
			if f.hasYear && canonical.hasYear && f.year != canonical.year {
				yearMismatch = true
				break
			}
		}

		for _, f := range group {
			if f.path == canonical.path {
				continue
			}
			payload := map[string]any{
				"title":     title,
				"src_path":  f.path,
				"dst_path":  canonical.path,
				"src_lib":   f.library,
				"dst_lib":   canonical.library,
				"kind":      kind,
			}
			switch {
			case yearMismatch:
				res.YearMismatches++
				enqueue(database.TaskKindYearMismatch, payload, 150)
			case !f.hasYear && canonical.hasYear:
				res.NoYearMerges++
				enqueue(database.TaskKindNoYearMerge, payload, 50)
			default:
				res.CrossVolumeDupes++
				enqueue(database.TaskKindMoveMerge, payload, 100)
			}
		}
	}
}

func chooseCanonical(group []folder) folder {
	best := group[0]
	for _, f := range group[1:] {
		if !best.hasYear && f.hasYear {
			best = f
			continue
		}
		// stable tiebreak: lexicographic library path so behaviour is
		// reproducible. Episode-count weighting is left for a future
		// pass once we cache counts in the DB.
		if best.hasYear == f.hasYear && f.library < best.library {
			best = f
		}
	}
	return best
}

func (e *Engine) detectOrphanSources(ctx context.Context, res *DetectResult, enqueue func(string, map[string]any, int)) {
	for _, dir := range e.cfg.WatchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if !ent.IsDir() {
				continue
			}
			path := filepath.Join(dir, ent.Name())
			if isEmptyDir(path) {
				res.OrphanSources++
				enqueue(database.TaskKindOrphanSource, map[string]any{
					"path": path,
				}, 80)
			}
		}
	}
}

func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		return false
	}
	return true
}

func (e *Engine) detectStuckSync(ctx context.Context, res *DetectResult, enqueue func(string, map[string]any, int)) {
	rows, err := e.db.DB().Query(`
		SELECT id, source, started_at FROM sync_log
		 WHERE status = 'running'
		   AND started_at < datetime('now', ?)
		`, fmt.Sprintf("-%d minutes", int(e.cfg.StuckSyncAfter.Minutes())))
	if err != nil {
		e.logf("warn", "stuck sync query failed err=%v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var source, started string
		if err := rows.Scan(&id, &source, &started); err != nil {
			continue
		}
		res.StuckSyncs++
		enqueue(database.TaskKindStuckSync, map[string]any{
			"sync_log_id": id,
			"source":      source,
			"started_at":  started,
		}, 90)
	}
}

// Drain processes pending housekeeping_tasks with bounded concurrency
// until the queue is empty or ctx is cancelled.
func (e *Engine) Drain(ctx context.Context) error {
	sem := make(chan struct{}, e.cfg.MaxConcurrentTasks)
	var wg sync.WaitGroup
	idleTicks := 0

	for {
		if ctx.Err() != nil {
			break
		}

		task, err := e.db.ClaimNextHousekeepingTask()
		if err != nil {
			e.logf("error", "claim task failed err=%v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if task == nil {
			idleTicks++
			if idleTicks >= 2 {
				break
			}
			select {
			case <-ctx.Done():
				break
			case <-time.After(2 * time.Second):
			}
			continue
		}
		idleTicks = 0

		sem <- struct{}{}
		wg.Add(1)
		go func(t *database.HousekeepingTask) {
			defer wg.Done()
			defer func() { <-sem }()
			runErr := e.executeTask(ctx, t)
			if completeErr := e.db.CompleteHousekeepingTask(t.ID, runErr, e.cfg.TaskRetryMax); completeErr != nil {
				e.logf("error", "complete task failed id=%d err=%v", t.ID, completeErr)
			}
			if e.cfg.TaskPauseBetween > 0 {
				time.Sleep(e.cfg.TaskPauseBetween)
			}
		}(task)
	}

	wg.Wait()
	return ctx.Err()
}

// executeTask dispatches a task to its handler. Flag-only kinds should
// never reach here (their initial status is 'flagged', not 'pending'),
// but we guard defensively.
func (e *Engine) executeTask(ctx context.Context, t *database.HousekeepingTask) error {
	if e.cfg.DryRun {
		e.logf("info", "[dry-run] would execute task id=%d kind=%s", t.ID, t.Kind)
		return nil
	}
	switch t.Kind {
	case database.TaskKindOrphanSource:
		return e.execOrphanSource(t)
	case database.TaskKindStuckSync:
		return e.execStuckSync(t)
	case database.TaskKindMoveMerge, database.TaskKindNoYearMerge:
		return e.execMergeMove(ctx, t)
	case database.TaskKindYearMismatch, database.TaskKindPollutedName, database.TaskKindSubdirMismatch:
		// Flag-only — should not be executed.
		return fmt.Errorf("flag-only task kind %q reached executor", t.Kind)
	default:
		return fmt.Errorf("unknown task kind %q", t.Kind)
	}
}

func (e *Engine) execOrphanSource(t *database.HousekeepingTask) error {
	pathAny, ok := t.Payload["path"]
	if !ok {
		return fmt.Errorf("missing path")
	}
	path, _ := pathAny.(string)
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if !isEmptyDir(path) {
		return fmt.Errorf("directory no longer empty: %s", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	e.logf("info", "removed orphan source dir path=%s", path)
	return nil
}

func (e *Engine) execStuckSync(t *database.HousekeepingTask) error {
	idAny, ok := t.Payload["sync_log_id"]
	if !ok {
		return fmt.Errorf("missing sync_log_id")
	}
	var id int64
	switch v := idAny.(type) {
	case float64:
		id = int64(v)
	case int64:
		id = v
	case int:
		id = int64(v)
	default:
		return fmt.Errorf("bad sync_log_id type %T", idAny)
	}
	_, err := e.db.DB().Exec(`
		UPDATE sync_log
		   SET status = 'error', completed_at = CURRENT_TIMESTAMP,
		       error_message = COALESCE(error_message, '') || ' [marked stuck by housekeeping]'
		 WHERE id = ? AND status = 'running'`, id)
	return err
}

func (e *Engine) logf(level, format string, args ...any) {
	if e.logger == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	switch level {
	case "error":
		e.logger.Error("housekeeping", msg, nil)
	case "warn":
		e.logger.Warn("housekeeping", msg)
	default:
		e.logger.Info("housekeeping", msg)
	}
}

// execMergeMove handles cross-volume duplicate merges and no-year merges.
// It walks every file under src_path and moves each one into dst_path,
// preserving the relative subdirectory structure (e.g. "Season 03"). After
// the walk completes successfully, the now-empty source tree is removed.
//
// Files that already exist at the destination with identical size are
// skipped (assumed duplicate). If sizes differ, the source copy is left
// in place and an error is returned so a human can adjudicate.
func (e *Engine) execMergeMove(ctx context.Context, t *database.HousekeepingTask) error {
srcAny, ok1 := t.Payload["src_path"]
dstAny, ok2 := t.Payload["dst_path"]
if !ok1 || !ok2 {
return fmt.Errorf("payload missing src_path/dst_path")
}
src, _ := srcAny.(string)
dst, _ := dstAny.(string)
if src == "" || dst == "" {
return fmt.Errorf("empty src or dst path")
}
if src == dst {
return fmt.Errorf("src == dst (%s)", src)
}
if e.transferer == nil {
return fmt.Errorf("no transferer configured")
}

srcInfo, err := os.Stat(src)
if err != nil {
return fmt.Errorf("stat src: %w", err)
}
if !srcInfo.IsDir() {
return fmt.Errorf("src not a directory: %s", src)
}
if _, err := os.Stat(dst); err != nil {
return fmt.Errorf("stat dst: %w", err)
}

moved := 0
skipped := 0
walkErr := filepath.Walk(src, func(path string, info os.FileInfo, werr error) error {
if werr != nil {
return werr
}
if ctx.Err() != nil {
return ctx.Err()
}
if info.IsDir() {
return nil
}
rel, err := filepath.Rel(src, path)
if err != nil {
return err
}
target := filepath.Join(dst, rel)

if existing, err := os.Stat(target); err == nil && !existing.IsDir() {
if existing.Size() == info.Size() {
if err := os.Remove(path); err != nil {
return fmt.Errorf("remove dup src %s: %w", path, err)
}
skipped++
return nil
}
return fmt.Errorf("size mismatch at %s (src=%d dst=%d) — manual review required",
target, info.Size(), existing.Size())
}

if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
}

res, err := e.transferer.Move(path, target, transfer.TransferOptions{
Timeout:   30 * time.Minute,
TargetUID: -1,
TargetGID: -1,
})
if err != nil {
return fmt.Errorf("move %s -> %s: %w", path, target, err)
}
if res != nil && res.Error != nil {
return fmt.Errorf("move %s -> %s: %w", path, target, res.Error)
}

if file, gerr := e.db.GetMediaFile(path); gerr == nil && file != nil {
_ = e.db.DeleteMediaFile(path)
file.Path = target
_ = e.db.UpsertMediaFile(file)
}

moved++
return nil
})

if walkErr != nil {
return walkErr
}

if err := removeIfEmptyTree(src); err != nil {
e.logf("warn", "merge: source not fully empty after move src=%s err=%v", src, err)
}

e.logf("info", "merged %d file(s), skipped %d duplicate(s) src=%s dst=%s",
moved, skipped, src, dst)
return nil
}

// removeIfEmptyTree removes a directory tree only if it contains no files.
// Empty subdirectories are removed bottom-up.
func removeIfEmptyTree(root string) error {
var dirs []string
err := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
if werr != nil {
return werr
}
if info.IsDir() {
dirs = append(dirs, path)
return nil
}
return fmt.Errorf("non-empty: %s", path)
})
if err != nil {
return err
}
for i := len(dirs) - 1; i >= 0; i-- {
_ = os.Remove(dirs[i])
}
return nil
}
