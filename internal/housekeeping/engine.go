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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
	"github.com/Nomadcxx/plex2jellyfin/internal/tmdb"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

// normalizeFolderTitle lowercases and collapses whitespace WITHOUT
// stripping trailing 4-digit numbers (which would conflate
// "Blade Runner" with "Blade Runner 2049"). Library folders are
// "Title (YYYY)" so the year is already separated by splitFolderName.
func normalizeFolderTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	// collapse internal whitespace and punctuation noise that varies
	// between Sonarr/Radarr renamers (e.g. " & " vs " and ").
	t = strings.ReplaceAll(t, " and ", " & ")
	t = strings.Join(strings.Fields(t), " ")
	return t
}

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
	verifier   *tmdb.Verifier
	cleanup    *service.CleanupService
	// registry is optional: when set, every executing task registers an
	// IPC op named "hk-task-<id>" and emits FrameProgress events that the
	// WebUI subscribes to for live progress bars. Nil-safe.
	registry   *ipc.OpRegistry
	rescanner  MediaRescanner
	translator *jellyfin.PathTranslator
}

// MediaRescanner triggers a targeted Jellyfin rescan of specific paths after a
// folder move, without a library-wide refresh. Satisfied by *jellyfin.Client.
type MediaRescanner interface {
	NotifyMediaUpdated(jellyfinPaths []string) error
}

// SetVerifier attaches a TMDB verifier so the detector can distinguish
// remakes from genuine duplicates before flagging year-mismatches.
func (e *Engine) SetVerifier(v *tmdb.Verifier) { e.verifier = v }

// SetOpRegistry wires the daemon's IPC OpRegistry into the engine so
// tasks can publish live progress frames consumable via SSE.
func (e *Engine) SetOpRegistry(r *ipc.OpRegistry) { e.registry = r }

func (e *Engine) SetMediaRescanner(r MediaRescanner)           { e.rescanner = r }
func (e *Engine) SetPathTranslator(t *jellyfin.PathTranslator) { e.translator = t }
func (e *Engine) renameWithFallback(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source for cross-device move %s: %w", src, err)
	}
	if srcInfo.IsDir() {
		return e.moveDirWithFallback(src, dst)
	}
	// Cross-device: fall back to transfer.Move
	if e.transferer == nil {
		return fmt.Errorf("EXDEV and no transferer available for %s -> %s", src, dst)
	}
	result, err := e.transferer.Move(src, dst, transfer.TransferOptions{})
	if err != nil {
		return fmt.Errorf("cross-device move %s -> %s: %w", src, dst, err)
	}
	if !result.Success {
		return fmt.Errorf("cross-device move failed: %v", result.Error)
	}
	if !result.SourceRemoved {
		return fmt.Errorf("cross-device move did not remove source: %s", src)
	}
	return nil
}

func (e *Engine) moveDirWithFallback(srcDir, dstDir string) error {
	if e.transferer == nil {
		return fmt.Errorf("EXDEV and no transferer available for %s -> %s", srcDir, dstDir)
	}
	if _, err := os.Stat(dstDir); err == nil && !isEmptyDir(dstDir) {
		return fmt.Errorf("destination directory already exists: %s", dstDir)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat destination directory %s: %w", dstDir, err)
	}

	if err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if rel == "." {
			return os.MkdirAll(dstDir, info.Mode().Perm())
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		result, err := e.transferer.Move(path, target, transfer.TransferOptions{})
		if err != nil {
			return fmt.Errorf("cross-device move %s -> %s: %w", path, target, err)
		}
		if !result.Success {
			return fmt.Errorf("cross-device move failed %s -> %s: %v", path, target, result.Error)
		}
		if !result.SourceRemoved {
			return fmt.Errorf("cross-device move did not remove source: %s", path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := removeIfEmptyTree(srcDir); err != nil {
		return fmt.Errorf("remove source dir %s: %w", srcDir, err)
	}
	return nil
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
	return &Engine{
		cfg:        cfg,
		db:         db,
		logger:     logger,
		transferer: tr,
		cleanup:    service.NewCleanupService(db),
	}
}

// DetectResult summarises one detect cycle.
type DetectResult struct {
	CrossVolumeDupes   int // duplicate workflow: low-confidence flags
	AutoDupes          int // duplicate workflow: high-confidence auto-delete tasks
	NoYearMerges       int
	YearMismatches     int
	VerifiedDistinct   int
	PollutedNames      int
	OrphanSources      int
	StuckSyncs         int
	FolderRenames      int
	ParserDriftRenames int
	Enqueued           int
	Skipped            int
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

	// File-level duplicate detection (movies + TV episodes), using the
	// same service.CleanupService the CLI uses. High-confidence groups
	// are queued for auto-delete-of-inferior; low-confidence groups are
	// flagged for human review.
	e.detectFileDuplicates(ctx, res, enqueue)

	// 5: orphan source dirs in watch directories
	e.detectOrphanSources(ctx, res, enqueue)

	// 6: stuck sync_log rows
	e.detectStuckSync(ctx, res, enqueue)

	// 7: parser drift repairs for successful movie imports created by
	// Plex2Jellyfin before parser fixes landed.
	e.detectParserDriftRepairs(ctx, res, enqueue)

	return res, nil
}

// folder represents a library subdirectory.
type folder struct {
	library  string
	path     string // absolute
	name     string // basename
	title    string // normalized
	year     string
	hasYear  bool
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
				title:    normalizeFolderTitle(title),
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

// detectDuplicateGroups groups folders by normalized title and emits
// **naming-workflow** tasks only (folder rename, year mismatch flag,
// no-year merge, polluted name flag). Cross-volume file-level duplicate
// detection is handled separately by detectFileDuplicates so it can
// reuse the consolidator's quality-aware logic.
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

		canonical := chooseCanonical(group)

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
				"title":    title,
				"src_path": f.path,
				"dst_path": canonical.path,
				"src_lib":  f.library,
				"dst_lib":  canonical.library,
				"kind":     kind,
			}
			switch {
			case yearMismatch:
				// Naming workflow: ALWAYS flag year mismatches for human
				// review. Even when the TMDB verifier reports "duplicate"
				// (which historically auto-promoted to move_merge and
				// caused the Clerks 1994→2022 incident), we now only
				// attach the verifier evidence to the flag — never act
				// on it automatically. Year mismatches are by definition
				// low confidence; remakes/sequels share titles.
				if e.verifier != nil && e.verifier.Available() {
					mk := tmdb.KindMovie
					if kind == "tv" {
						mk = tmdb.KindSeries
					}
					if vr := e.verifier.Verify(ctx, mk,
						title, f.year, title, canonical.year); vr != nil {
						payload["verification"] = vr
						if vr.Verdict == "distinct" {
							res.VerifiedDistinct++
							continue // legitimate remakes — don't even flag
						}
					}
				}
				res.YearMismatches++
				enqueue(database.TaskKindYearMismatch, payload, 150)
			case !f.hasYear && canonical.hasYear:
				// Naming workflow: a no-year folder that has a clear
				// year-bearing twin in the same library is safe to
				// merge in-place (existing no_year_merge behaviour).
				res.NoYearMerges++
				enqueue(database.TaskKindNoYearMerge, payload, 50)
			case sameLibraryCaseFoldOnly(f, canonical):
				// Naming workflow: same library, identical content
				// modulo case/whitespace — auto rename in place. Risk-
				// free because the canonical folder already exists and
				// we're just folding the duplicate into it.
				res.FolderRenames++
				enqueue(database.TaskKindFolderRename, payload, 75)
			default:
				// Cross-volume same-title pair: do NOT enqueue a folder
				// merge here. The duplicate workflow (detectFileDuplicates
				// below) will pick this up at the file level via the
				// consolidator's quality-aware delete logic.
			}
		}
	}
}

// sameLibraryCaseFoldOnly returns true when two folders live in the
// same library root and their basenames differ only by case/whitespace.
// Used to identify the risk-free folder-rename case (naming workflow).
func sameLibraryCaseFoldOnly(a, b folder) bool {
	if a.library != b.library {
		return false
	}
	na := strings.ToLower(strings.Join(strings.Fields(a.name), " "))
	nb := strings.ToLower(strings.Join(strings.Fields(b.name), " "))
	return na == nb && a.name != b.name
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

// VerifyFlaggedResult summarises a sweep of flagged year_mismatch tasks
// through the TMDB verifier.
type VerifyFlaggedResult struct {
	Scanned   int
	Distinct  int
	Duplicate int
	Unknown   int
	Errors    int
}

// VerifyFlagged re-runs the verifier against every flagged year_mismatch
// task. Tasks identified as legitimate remakes are downgraded to
// 'skipped' (visible in the WebUI but inert); tasks identified as real
// cross-volume duplicates are upgraded to a pending move_merge.
func (e *Engine) VerifyFlagged(ctx context.Context) (*VerifyFlaggedResult, error) {
	res := &VerifyFlaggedResult{}
	if e.verifier == nil || !e.verifier.Available() {
		return res, fmt.Errorf("verifier unavailable: configure jellyfin or tmdb api key")
	}
	tasks, err := e.db.ListHousekeepingTasksByKind(database.TaskKindYearMismatch, database.TaskStatusFlagged, 0)
	if err != nil {
		return res, err
	}
	for _, t := range tasks {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		res.Scanned++
		title, _ := t.Payload["title"].(string)
		kind, _ := t.Payload["kind"].(string)
		srcYear := folderYearFromPath(t.Payload["src_path"])
		dstYear := folderYearFromPath(t.Payload["dst_path"])
		mk := tmdb.KindMovie
		if kind == "tv" {
			mk = tmdb.KindSeries
		}
		vr := e.verifier.Verify(ctx, mk, title, srcYear, title, dstYear)
		if vr == nil {
			res.Errors++
			continue
		}
		t.Payload["verification"] = vr
		switch vr.Verdict {
		case "distinct":
			res.Distinct++
			if err := e.db.UpdateHousekeepingTask(t.ID, database.TaskKindYearMismatch, database.TaskStatusSkipped, t.Payload); err != nil {
				res.Errors++
			}
		case "duplicate":
			res.Duplicate++
			if err := e.db.UpdateHousekeepingTask(t.ID, database.TaskKindMoveMerge, database.TaskStatusPending, t.Payload); err != nil {
				res.Errors++
			}
		default:
			res.Unknown++
			// keep flagged but persist verification for UI display
			if err := e.db.UpdateHousekeepingTask(t.ID, database.TaskKindYearMismatch, database.TaskStatusFlagged, t.Payload); err != nil {
				res.Errors++
			}
		}
	}
	return res, nil
}

// VerifyTask re-runs the verifier on a single flagged year_mismatch task.
// Returns the resulting verdict plus whether the task was updated.
func (e *Engine) VerifyTask(ctx context.Context, id int64) (*tmdb.VerifyResult, error) {
	if e.verifier == nil || !e.verifier.Available() {
		return nil, fmt.Errorf("verifier unavailable: configure jellyfin or tmdb api key")
	}
	t, err := e.db.GetHousekeepingTask(id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("task %d not found", id)
	}
	if t.Kind != database.TaskKindYearMismatch {
		return nil, fmt.Errorf("task %d is not a year_mismatch", id)
	}
	title, _ := t.Payload["title"].(string)
	kind, _ := t.Payload["kind"].(string)
	srcYear := folderYearFromPath(t.Payload["src_path"])
	dstYear := folderYearFromPath(t.Payload["dst_path"])
	mk := tmdb.KindMovie
	if kind == "tv" {
		mk = tmdb.KindSeries
	}
	vr := e.verifier.Verify(ctx, mk, title, srcYear, title, dstYear)
	if vr == nil {
		return nil, fmt.Errorf("verifier returned no result")
	}
	t.Payload["verification"] = vr
	switch vr.Verdict {
	case "distinct":
		_ = e.db.UpdateHousekeepingTask(t.ID, database.TaskKindYearMismatch, database.TaskStatusSkipped, t.Payload)
	case "duplicate":
		_ = e.db.UpdateHousekeepingTask(t.ID, database.TaskKindMoveMerge, database.TaskStatusPending, t.Payload)
	default:
		_ = e.db.UpdateHousekeepingTask(t.ID, database.TaskKindYearMismatch, database.TaskStatusFlagged, t.Payload)
	}
	return vr, nil
}

// folderYearFromPath extracts a 4-digit year from a folder path like
// "/srv/Movies/Solaris (1972)" → "1972". Returns "" if not found.
func folderYearFromPath(v any) string {
	s, _ := v.(string)
	if s == "" {
		return ""
	}
	base := filepath.Base(s)
	// look for "(YYYY)"
	i := strings.LastIndex(base, "(")
	j := strings.LastIndex(base, ")")
	if i >= 0 && j > i+4 {
		inner := base[i+1 : j]
		if len(inner) == 4 && isAllDigits(inner) {
			return inner
		}
	}
	return ""
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

func (e *Engine) detectParserDriftRepairs(ctx context.Context, res *DetectResult, enqueue func(string, map[string]any, int)) {
	if e.db == nil {
		return
	}
	const parserDriftLookback = 60 * 24 * time.Hour
	rows, err := e.db.QueryRecentSuccessfulMovieImports(parserDriftLookback, 500)
	if err != nil {
		e.logf("warn", "parser drift query failed err=%v", err)
	} else {
		for _, d := range rows {
			if ctx.Err() != nil {
				return
			}
			srcPath, dstPath, ok := e.parserDriftMovieRename(d)
			if !ok {
				continue
			}
			res.ParserDriftRenames++
			enqueue(database.TaskKindParserDriftRename, map[string]any{
				"parse_decision_id": d.ID,
				"src_path":          srcPath,
				"dst_path":          dstPath,
				"source_filename":   d.SourceFilename,
			}, 70)
		}
	}

	tvRows, err := e.db.QueryRecentSuccessfulTVImports(parserDriftLookback, 500)
	if err != nil {
		e.logf("warn", "parser drift tv query failed err=%v", err)
		return
	}
	for _, d := range tvRows {
		if ctx.Err() != nil {
			return
		}
		srcPath, dstPath, ok := e.parserDriftTVRename(d)
		if !ok {
			continue
		}
		res.ParserDriftRenames++
		enqueue(database.TaskKindParserDriftTVRename, map[string]any{
			"parse_decision_id": d.ID,
			"src_path":          srcPath,
			"dst_path":          dstPath,
			"source_filename":   d.SourceFilename,
		}, 70)
	}
}

func (e *Engine) parserDriftMovieRename(d *database.ParseDecision) (srcPath, dstPath string, ok bool) {
	if d == nil || d.SourceFilename == "" || d.TargetPath == "" {
		return "", "", false
	}
	if d.ExistingMatchMethod == "verifier" {
		// This row's name came from the TMDB verifier, not the parser. The
		// parser cannot re-derive it, so drift detection must not fight the
		// corrector by renaming back to the parser's wrong title.
		return "", "", false
	}
	libRoot := containingLibrary(d.TargetPath, e.cfg.MovieLibraries)
	if libRoot == "" {
		return "", "", false
	}
	info, err := naming.ParseMovieName(d.SourceFilename)
	if err != nil || info == nil || info.Title == "" {
		return "", "", false
	}
	cleanName := naming.NormalizeMediaName(info.Title, info.Year)
	ext := filepath.Ext(d.TargetPath)
	if ext == "" {
		ext = filepath.Ext(d.SourceFilename)
	}
	if cleanName == "" || ext == "" {
		return "", "", false
	}
	srcPath = filepath.Clean(d.TargetPath)
	dstPath = filepath.Join(libRoot, cleanName, cleanName+ext)
	if srcPath == filepath.Clean(dstPath) {
		return "", "", false
	}
	srcExists := false
	if _, err := os.Stat(srcPath); err == nil {
		srcExists = true
	} else if !os.IsNotExist(err) {
		return "", "", false
	}
	dstExists := false
	if _, err := os.Stat(dstPath); err == nil {
		dstExists = true
	} else if !os.IsNotExist(err) {
		return "", "", false
	}
	if srcExists && dstExists {
		return "", "", false
	}
	if srcExists {
		if _, err := os.Stat(filepath.Dir(srcPath)); err != nil {
			return "", "", false
		}
		if _, err := os.Stat(filepath.Dir(dstPath)); err == nil {
			if !isEmptyDir(filepath.Dir(dstPath)) {
				return "", "", false
			}
		} else if !os.IsNotExist(err) {
			return "", "", false
		}
	}
	if !srcExists && !dstExists {
		return "", "", false
	}
	return srcPath, dstPath, true
}

func (e *Engine) parserDriftTVRename(d *database.ParseDecision) (srcPath, dstPath string, ok bool) {
	if d == nil || d.SourceFilename == "" || d.TargetPath == "" {
		return "", "", false
	}
	libRoot := containingLibrary(d.TargetPath, e.cfg.TVLibraries)
	if libRoot == "" {
		return "", "", false
	}
	info, err := naming.ParseTVShowName(d.SourceFilename)
	if err != nil || info == nil || info.Title == "" || info.Season <= 0 || info.Episode <= 0 {
		return "", "", false
	}
	if !tvParserDriftWasReleaseYearAfterEpisode(d, libRoot) {
		return "", "", false
	}
	showName := naming.NormalizeMediaName(info.Title, info.Year)
	ext := filepath.Ext(d.TargetPath)
	if ext == "" {
		ext = filepath.Ext(d.SourceFilename)
	}
	if showName == "" || ext == "" {
		return "", "", false
	}
	episodeName := naming.FormatTVEpisodeFilenameFromInfo(info, strings.TrimPrefix(ext, "."))
	if episodeName == "" {
		return "", "", false
	}
	srcPath = filepath.Clean(d.TargetPath)
	dstPath = filepath.Join(libRoot, showName, naming.FormatSeasonFolder(info.Season), episodeName)
	if srcPath == filepath.Clean(dstPath) {
		return "", "", false
	}
	srcExists := false
	if _, err := os.Stat(srcPath); err == nil {
		srcExists = true
	} else if !os.IsNotExist(err) {
		return "", "", false
	}
	dstExists := false
	if _, err := os.Stat(dstPath); err == nil {
		dstExists = true
	} else if !os.IsNotExist(err) {
		return "", "", false
	}
	if srcExists && dstExists {
		return "", "", false
	}
	if !srcExists && !dstExists {
		return "", "", false
	}
	return srcPath, dstPath, true
}

var tvEpisodeMarkerReleaseYearRegex = regexp.MustCompile(`(?i)(?:[Ss]\d{1,2}[Ee]\d{1,2}|\d{1,2}x\d{1,2}|EP[.\-_ ]?\d{2,5}).*\b((?:19|20)\d{2})\b`)

func tvParserDriftWasReleaseYearAfterEpisode(d *database.ParseDecision, libRoot string) bool {
	if d == nil || d.ParsedYear == nil {
		return false
	}
	if d.MetadataState != "series_unidentified" {
		return false
	}
	if d.JellyfinIdentified != nil && *d.JellyfinIdentified {
		return false
	}
	releaseYear, ok := releaseYearAfterEpisodeMarker(d.SourceFilename)
	if !ok || releaseYear != *d.ParsedYear {
		return false
	}
	targetYear := tvShowFolderYear(d.TargetPath, libRoot)
	return targetYear == "" || targetYear == strconv.Itoa(*d.ParsedYear)
}

func releaseYearAfterEpisodeMarker(filename string) (int, bool) {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	matches := tvEpisodeMarkerReleaseYearRegex.FindAllStringSubmatch(baseName, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if len(matches[i]) < 2 {
			continue
		}
		year, err := strconv.Atoi(matches[i][1])
		if err != nil {
			continue
		}
		switch year {
		case 1280, 1440, 1920, 2160:
			continue
		default:
			return year, true
		}
	}
	return 0, false
}

func tvShowFolderYear(targetPath, libRoot string) string {
	rel, err := filepath.Rel(filepath.Clean(libRoot), filepath.Clean(targetPath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	return folderYearFromPath(parts[0])
}

func containingLibrary(path string, libs []string) string {
	cleanPath := filepath.Clean(path)
	for _, lib := range libs {
		cleanLib := filepath.Clean(lib)
		if cleanPath == cleanLib || strings.HasPrefix(cleanPath, cleanLib+string(filepath.Separator)) {
			return cleanLib
		}
	}
	return ""
}

// detectFileDuplicates finds duplicate movies/episodes at the file level
// (not folder level) using the same service.CleanupService the CLI uses.
// For each duplicate group it classifies confidence:
//   - "high" → enqueue TaskKindConsolidateDuplicate (auto-deletes the
//     inferior copies via the consolidator's delete logic).
//   - "low"  → enqueue TaskKindCrossVolumeDuplicate (flag for human
//     review in the WebUI).
//
// This is the entry point for the **duplicate-removal workflow** —
// distinct from the **naming workflow** in detectDuplicateGroups and
// from the **TV-scatter consolidation workflow** (future).
func (e *Engine) detectFileDuplicates(ctx context.Context, res *DetectResult, enqueue func(string, map[string]any, int)) {
	if e.cleanup == nil {
		return
	}
	analysis, err := e.cleanup.AnalyzeDuplicates()
	if err != nil {
		e.logf("warn", "duplicate analysis failed err=%v", err)
		return
	}
	for i := range analysis.Groups {
		if ctx.Err() != nil {
			return
		}
		group := &analysis.Groups[i]
		if len(group.Files) < 2 {
			continue
		}

		// Filter parser-failure groups. Titles like "season", "season1",
		// pure digits, or empties are not real duplicates — they're
		// folders the parser couldn't classify. Grouping them auto-
		// merges unrelated shows into one bogus duplicate group.
		if isParseFailureTitle(group.Title) {
			res.Skipped++
			continue
		}

		level, reasons := e.cleanup.GroupConfidence(*group)

		// Payload carries the natural key so the executor can re-fetch
		// the group at execute-time (state may have changed since
		// detection — files deleted, re-scanned, etc.).
		payload := map[string]any{
			"group_id":         group.ID,
			"media_type":       group.MediaType,
			"normalized_title": group.Title,
			"reclaimable":      group.ReclaimableBytes,
			"file_count":       len(group.Files),
			"best_file_id":     group.BestFileID,
			"confidence":       level,
		}
		if group.Year != nil {
			payload["year"] = *group.Year
		}
		if group.Season != nil {
			payload["season"] = *group.Season
		}
		if group.Episode != nil {
			payload["episode"] = *group.Episode
		}
		if len(reasons) > 0 {
			payload["reasons"] = reasons
		}

		switch level {
		case service.ConfidenceHigh:
			res.AutoDupes++
			enqueue(database.TaskKindConsolidateDuplicate, payload, 60)
		default:
			res.CrossVolumeDupes++
			enqueue(database.TaskKindCrossVolumeDuplicate, payload, 140)
		}
	}
}

// isParseFailureTitle returns true if the normalized title looks like
// a parser failure — a string with no information content. These
// false-grouping seeds (e.g. "season", "season1", "season01",
// "specials", "extras", "1080", "720", or pure numbers) cause
// unrelated shows to be lumped into one bogus "duplicate" group.
func isParseFailureTitle(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	if len(t) <= 1 {
		return true
	}
	if isAllDigitsOrPunct(t) {
		return true
	}
	switch t {
	case "season", "specials", "extras", "featurettes",
		"behindthescenes", "deletedscenes", "interviews",
		"trailers", "scenes", "shorts", "1080p", "720p", "2160p",
		"4k", "uhd", "bluray", "remux":
		return true
	}
	if strings.HasPrefix(t, "season") {
		rest := strings.TrimPrefix(t, "season")
		if rest == "" || isAllDigitsOrPunct(rest) {
			return true
		}
	}
	if strings.HasPrefix(t, "disc") {
		rest := strings.TrimPrefix(t, "disc")
		if rest == "" || isAllDigitsOrPunct(rest) {
			return true
		}
	}
	return false
}

func isAllDigitsOrPunct(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '.' || r == '-' || r == '_' || r == ' ' {
			continue
		}
		return false
	}
	return true
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
		// Naming-workflow folder merges. TaskKindMoveMerge is no longer
		// enqueued by detect (the duplicate workflow handles that via
		// TaskKindConsolidateDuplicate) but the handler is retained so
		// any legacy queued or human-approved merges still execute.
		return e.execMergeMove(ctx, t)
	case database.TaskKindFolderRename:
		// Naming workflow: in-place rename of a folder whose name only
		// differs from its canonical twin by case/whitespace. Performed
		// as a folder merge because the canonical folder already exists
		// in the same library (no cross-volume work).
		return e.execMergeMove(ctx, t)
	case database.TaskKindParserDriftRename:
		// Naming workflow: Plex2Jellyfin previously organized a movie to a
		// path derived from an older parser. The current parser now derives
		// a better same-library path, so rename the folder/file in place.
		return e.execParserDriftRename(t)
	case database.TaskKindParserDriftTVRename:
		// Naming workflow: Plex2Jellyfin previously organized a TV episode to
		// a path derived from an older parser. The current parser now
		// derives a better same-library episode path, so move that file in
		// place and reconcile the DB row.
		return e.execParserDriftTVRename(t)
	case database.TaskKindVerifierRename:
		// Phase 2 corrector: rename a movie folder to a TMDB-verified title
		// that the parser could not derive. Writes the verifier title (never
		// re-parses) and stamps the row so drift detection ignores it.
		return e.execVerifierRename(t)
	case database.TaskKindConsolidateDuplicate:
		// Duplicate workflow: delete the inferior copies of a known
		// high-confidence duplicate group via the same logic the CLI
		// uses (service.CleanupService.DeleteDuplicateFiles).
		return e.execConsolidateDuplicate(ctx, t)
	case database.TaskKindYearMismatch, database.TaskKindCrossVolumeDuplicate:
		// Flag kinds. Reaching the executor means a human approved them
		// in the WebUI; treat them as duplicate-resolution requests.
		return e.execConsolidateDuplicate(ctx, t)
	case database.TaskKindPollutedName, database.TaskKindSubdirMismatch:
		// These have no deterministic target — a human must rename
		// manually; reaching the executor is an error.
		return fmt.Errorf("flag-only task kind %q requires manual action", t.Kind)
	default:
		return fmt.Errorf("unknown task kind %q", t.Kind)
	}
}

func (e *Engine) recordRepairEvent(action, safetyClass string, confidence float64, src, dst, outcome, errMsg string, evidence map[string]any) {
	if e == nil || e.db == nil {
		return
	}
	evidenceJSON := ""
	if len(evidence) > 0 {
		if b, err := json.Marshal(evidence); err == nil {
			evidenceJSON = string(b)
		}
	}
	_, _ = e.db.InsertRepairEvent(database.RepairEvent{
		EventAt:      time.Now().UTC(),
		Action:       action,
		SafetyClass:  safetyClass,
		Confidence:   confidence,
		SourcePath:   src,
		TargetPath:   dst,
		Outcome:      outcome,
		Error:        errMsg,
		LLMConsulted: false,
		EvidenceJSON: evidenceJSON,
	})
}

// moveOrganizedFolder performs the on-disk half of a folder rename src->dst
// with the same safety checks as parser-drift rename: refuses if both or
// neither exist, requires a folder-level rename, tolerates an already-moved
// source (src missing, dst exists => on-disk no-op). Re-points the triggering
// file's media_files row.
func (e *Engine) moveOrganizedFolder(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return fmt.Errorf("src == dst (%s)", src)
	}
	srcDir := filepath.Dir(src)
	dstDir := filepath.Dir(dst)
	if srcDir == dstDir {
		return fmt.Errorf("rename requires folder rename: %s", srcDir)
	}
	srcExists := false
	if _, err := os.Stat(src); err == nil {
		srcExists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat src: %w", err)
	}
	dstExists := false
	if _, err := os.Stat(dst); err == nil {
		dstExists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat dst: %w", err)
	}
	if srcExists && dstExists {
		return fmt.Errorf("source and destination both exist: %s -> %s", src, dst)
	}
	if !srcExists && !dstExists {
		// Self-heal a partial move: a previous run renamed the folder but
		// failed the inner-file rename, leaving the file at the old basename
		// inside dstDir. Complete that rename instead of erroring out.
		movedPath := filepath.Join(dstDir, filepath.Base(src))
		if filepath.Base(src) != filepath.Base(dst) {
			if _, err := os.Stat(movedPath); err == nil {
				if err := e.renameWithFallback(movedPath, dst); err != nil {
					return fmt.Errorf("rename file %s -> %s: %w", movedPath, dst, err)
				}
				dstExists = true
			}
		}
		if !dstExists {
			return fmt.Errorf("source and destination both missing: %s -> %s", src, dst)
		}
	}
	if srcExists {
		dstDirExists := false
		if _, err := os.Stat(dstDir); err == nil {
			dstDirExists = true
			if !isEmptyDir(dstDir) {
				return fmt.Errorf("destination directory already exists: %s", dstDir)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat dst dir: %w", err)
		}
		if dstDirExists {
			if err := e.renameWithFallback(src, dst); err != nil {
				return fmt.Errorf("rename file %s -> %s: %w", src, dst, err)
			}
			if err := os.Remove(srcDir); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove source dir %s: %w", srcDir, err)
			}
		} else {
			if err := e.renameWithFallback(srcDir, dstDir); err != nil {
				return fmt.Errorf("rename folder %s -> %s: %w", srcDir, dstDir, err)
			}
			movedPath := filepath.Join(dstDir, filepath.Base(src))
			if filepath.Base(src) != filepath.Base(dst) {
				if err := e.renameWithFallback(movedPath, dst); err != nil {
					return fmt.Errorf("rename file %s -> %s: %w", movedPath, dst, err)
				}
			}
		}
	}

	if file, err := e.db.GetMediaFile(src); err == nil && file != nil {
		_ = e.db.DeleteMediaFile(src)
		file.Path = dst
		file.LibraryRoot = containingLibrary(dst, e.cfg.MovieLibraries)
		_ = e.db.UpsertMediaFile(file)
	}
	return nil
}

func (e *Engine) execParserDriftRename(t *database.HousekeepingTask) (err error) {
	src, _ := t.Payload["src_path"].(string)
	dst, _ := t.Payload["dst_path"].(string)
	defer func() {
		if err != nil {
			e.recordRepairEvent(database.TaskKindParserDriftRename, "auto_safe", 0.95, src, dst, "failed", err.Error(), t.Payload)
		}
	}()
	if src == "" || dst == "" {
		return fmt.Errorf("payload missing src_path/dst_path")
	}
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return fmt.Errorf("src == dst (%s)", src)
	}
	if err := e.moveOrganizedFolder(src, dst); err != nil {
		return err
	}

	if id, ok := payloadInt64(t.Payload, "parse_decision_id"); ok && id > 0 {
		var preservedTargetAt *time.Time
		if existing, err := e.db.GetDecision(id); err == nil && existing != nil {
			preservedTargetAt = existing.TargetAt
		}
		if preservedTargetAt == nil {
			// Never NULL out target_at: it drives the metadata 48h grace clock.
			// Fall back to now when the row can't be re-read.
			now := time.Now().UTC()
			preservedTargetAt = &now
		}
		_ = e.db.UpdateOrganize(id, database.OrganizeUpdate{
			TargetPath:      dst,
			TargetAt:        preservedTargetAt,
			OrganizeOutcome: "success",
		})
		if sourceFilename, _ := t.Payload["source_filename"].(string); sourceFilename != "" {
			if info, tokens, err := naming.ParseMovieNameVerbose(sourceFilename); err == nil && info != nil {
				var parsedYear *int
				if info.Year != "" {
					year := 0
					if _, err := fmt.Sscanf(info.Year, "%d", &year); err == nil {
						parsedYear = &year
					}
				}
				tokenJSON := ""
				if b, err := json.Marshal(tokens); err == nil {
					tokenJSON = string(b)
				}
				_ = e.db.UpdateParse(id, database.ParseUpdate{
					ParseMethod:          "regex",
					ParsedTitle:          info.Title,
					ParsedYear:           parsedYear,
					ParserStrippedTokens: tokenJSON,
					MediaTypeGuessed:     "movie",
				})
			}
		}
	}

	e.rescanAfterMove(dst)
	e.logf("info", "parser-drift renamed src=%s dst=%s", src, dst)
	e.recordRepairEvent(database.TaskKindParserDriftRename, "auto_safe", 0.95, src, dst, "success", "", t.Payload)
	return nil
}

func (e *Engine) execVerifierRename(t *database.HousekeepingTask) (err error) {
	src, _ := t.Payload["src_path"].(string)
	dst, _ := t.Payload["dst_path"].(string)
	newTitle, _ := t.Payload["new_title"].(string)
	newYear, _ := t.Payload["new_year"].(string)
	tmdbID, _ := t.Payload["tmdb_id"].(string)
	evidence := map[string]any{"tmdb_id": tmdbID, "new_title": newTitle, "new_year": newYear}
	defer func() {
		if err != nil {
			e.recordRepairEvent(database.TaskKindVerifierRename, "auto_verified", 0.9, src, dst, "failed", err.Error(), evidence)
		}
	}()
	if src == "" || dst == "" || newTitle == "" {
		return fmt.Errorf("payload missing src_path/dst_path/new_title")
	}

	srcDir := filepath.Dir(filepath.Clean(src))
	dstDir := filepath.Dir(filepath.Clean(dst))

	// DB-first ordering: a DB failure prevents the filesystem move, and a
	// failed move after DB updates leaves rows pointing at the destination,
	// which the metadata reconciler surfaces as target_file_missing.
	// Retries are safe: already-repointed rows no longer match the srcDir
	// prefix, and moveOrganizedFolder treats "src gone, dst present" as done.
	if err := e.db.RepointMediaFilesUnderFolder(srcDir, dstDir); err != nil {
		return fmt.Errorf("repoint media_files: %w", err)
	}

	rows, err := e.db.QueryDecisionsUnderFolder(srcDir)
	if err != nil {
		return fmt.Errorf("query decisions under folder: %w", err)
	}
	var parsedYear *int
	if newYear != "" {
		y := 0
		if _, scanErr := fmt.Sscanf(newYear, "%d", &y); scanErr == nil {
			parsedYear = &y
		}
	}
	for _, row := range rows {
		newTarget := strings.Replace(row.TargetPath, srcDir, dstDir, 1)
		// row.TargetAt passes through unchanged (nil == already NULL in DB,
		// so the write is idempotent and never fabricates an import time).
		if err := e.db.UpdateOrganize(row.ID, database.OrganizeUpdate{
			TargetPath:          newTarget,
			TargetAt:            row.TargetAt,
			ExistingMatchMethod: "verifier",
			OrganizeOutcome:     "success",
		}); err != nil {
			return fmt.Errorf("update organize row %d: %w", row.ID, err)
		}
		if err := e.db.UpdateParse(row.ID, database.ParseUpdate{
			ParseMethod:      "verifier",
			ParsedTitle:      newTitle,
			ParsedYear:       parsedYear,
			MediaTypeGuessed: "movie",
		}); err != nil {
			return fmt.Errorf("update parse row %d: %w", row.ID, err)
		}
	}

	if err := e.moveOrganizedFolder(src, dst); err != nil {
		return err
	}

	e.rescanAfterMove(dst)
	e.logf("info", "verifier renamed src=%s dst=%s title=%q tmdb=%s", src, dst, newTitle, tmdbID)
	e.recordRepairEvent(database.TaskKindVerifierRename, "auto_verified", 0.9, src, dst, "success", "", evidence)
	return nil
}

// rescanAfterMove issues a targeted Jellyfin rescan for the destination path
// after a folder move. Nil-safe: does nothing when no rescanner is configured.
// The path is translated to Jellyfin's view; a library-wide refresh is never
// used.
func (e *Engine) rescanAfterMove(daemonDstPath string) {
	if e == nil || e.rescanner == nil {
		return
	}
	folder := filepath.Dir(daemonDstPath)
	jellyfinPath := folder
	if e.translator != nil {
		jellyfinPath = e.translator.DaemonToJellyfin(folder)
	}
	if err := e.rescanner.NotifyMediaUpdated([]string{jellyfinPath}); err != nil {
		e.logf("warn", "targeted rescan failed path=%s err=%v", jellyfinPath, err)
	}
}

func (e *Engine) execParserDriftTVRename(t *database.HousekeepingTask) (err error) {
	src, _ := t.Payload["src_path"].(string)
	dst, _ := t.Payload["dst_path"].(string)
	defer func() {
		if err != nil {
			e.recordRepairEvent(database.TaskKindParserDriftTVRename, "auto_safe", 0.95, src, dst, "failed", err.Error(), t.Payload)
		}
	}()
	if src == "" || dst == "" {
		return fmt.Errorf("payload missing src_path/dst_path")
	}
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return fmt.Errorf("src == dst (%s)", src)
	}
	if id, ok := payloadInt64(t.Payload, "parse_decision_id"); ok && id > 0 {
		d, err := e.db.GetDecision(id)
		if err != nil {
			return fmt.Errorf("get parse decision %d: %w", id, err)
		}
		expectedSrc, expectedDst, eligible := e.parserDriftTVRename(d)
		if !eligible {
			return fmt.Errorf("tv parser drift task no longer eligible for decision %d", id)
		}
		if filepath.Clean(expectedSrc) != src || filepath.Clean(expectedDst) != dst {
			return fmt.Errorf("tv parser drift payload mismatch for decision %d", id)
		}
	}

	srcExists := false
	if _, err := os.Stat(src); err == nil {
		srcExists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat src: %w", err)
	}
	dstExists := false
	if _, err := os.Stat(dst); err == nil {
		dstExists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat dst: %w", err)
	}
	if srcExists && dstExists {
		return fmt.Errorf("source and destination both exist: %s -> %s", src, dst)
	}
	if !srcExists && !dstExists {
		return fmt.Errorf("source and destination both missing: %s -> %s", src, dst)
	}

	if srcExists {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := e.renameWithFallback(src, dst); err != nil {
			return fmt.Errorf("rename file %s -> %s: %w", src, dst, err)
		}
		removeDirIfEmpty(filepath.Dir(src))
		removeDirIfEmpty(filepath.Dir(filepath.Dir(src)))
	}

	if file, err := e.db.GetMediaFile(src); err == nil && file != nil {
		_ = e.db.DeleteMediaFile(src)
		file.Path = dst
		file.LibraryRoot = containingLibrary(dst, e.cfg.TVLibraries)
		_ = e.db.UpsertMediaFile(file)
	}

	if id, ok := payloadInt64(t.Payload, "parse_decision_id"); ok && id > 0 {
		now := time.Now().UTC()
		_ = e.db.UpdateOrganize(id, database.OrganizeUpdate{
			TargetPath:      dst,
			TargetAt:        &now,
			OrganizeOutcome: "success",
		})
		if sourceFilename, _ := t.Payload["source_filename"].(string); sourceFilename != "" {
			if info, tokens, err := naming.ParseTVShowNameVerbose(sourceFilename); err == nil && info != nil {
				var parsedYear *int
				if info.Year != "" {
					year := 0
					if _, err := fmt.Sscanf(info.Year, "%d", &year); err == nil {
						parsedYear = &year
					}
				}
				season := info.Season
				episode := info.Episode
				tokenJSON := ""
				if b, err := json.Marshal(tokens); err == nil {
					tokenJSON = string(b)
				}
				_ = e.db.UpdateParse(id, database.ParseUpdate{
					ParseMethod:          "regex",
					ParsedTitle:          info.Title,
					ParsedYear:           parsedYear,
					ParsedSeason:         &season,
					ParsedEpisode:        &episode,
					ParserStrippedTokens: tokenJSON,
					MediaTypeGuessed:     "tv",
				})
			}
		}
	}

	e.logf("info", "parser-drift-tv renamed src=%s dst=%s", src, dst)
	e.recordRepairEvent(database.TaskKindParserDriftTVRename, "auto_safe", 0.95, src, dst, "success", "", t.Payload)
	return nil
}

func removeDirIfEmpty(path string) {
	if path == "" || path == "." || path == string(filepath.Separator) {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return
	}
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case float64:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	default:
		return 0, false
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

// execConsolidateDuplicate resolves a duplicate group by deleting its
// inferior copies, keeping the file flagged as `BestFile`. Reuses the
// same service.CleanupService logic the CLI's `plex2jellyfin duplicates
// execute` uses, so the daemon and CLI cannot diverge on what "the
// inferior copy" means.
//
// Re-fetches the group at execute time using the natural key persisted
// in the task payload (group_id + media_type + title + year[+S/E]) so
// that any state drift since detection (manual deletes, rescans, file
// regenerations) is reflected. If the group no longer exists or no
// longer qualifies as high-confidence, the task is treated as a no-op
// success — the queue should not be poisoned by stale tasks.
func (e *Engine) execConsolidateDuplicate(ctx context.Context, t *database.HousekeepingTask) error {
	if e.cleanup == nil {
		return fmt.Errorf("cleanup service not configured")
	}
	mediaType, _ := t.Payload["media_type"].(string)
	title, _ := t.Payload["normalized_title"].(string)
	if mediaType == "" || title == "" {
		return fmt.Errorf("payload missing media_type/normalized_title")
	}
	year := payloadIntPtr(t.Payload, "year")
	season := payloadIntPtr(t.Payload, "season")
	episode := payloadIntPtr(t.Payload, "episode")

	group, err := e.cleanup.FindDuplicateGroup(mediaType, title, year, season, episode)
	if err != nil {
		return fmt.Errorf("lookup duplicate group: %w", err)
	}
	if group == nil {
		e.logf("info", "duplicate group already resolved id=%d title=%q — no-op", t.ID, title)
		return nil
	}

	// Re-check confidence at execute time. A group that was high-conf
	// at detection may have changed (e.g. a rescan picked up a stale
	// record). For approved-flag tasks (year_mismatch, cross_volume_*)
	// the human has already authorised the action so we don't re-gate.
	if t.Kind == database.TaskKindConsolidateDuplicate {
		if level, reasons := e.cleanup.GroupConfidence(*group); level != service.ConfidenceHigh {
			return fmt.Errorf("group no longer high-confidence at execute time: %v", reasons)
		}
	}

	deleted, reclaimed, derr := e.cleanup.ResolveDuplicateGroup(group)
	if derr != nil {
		return fmt.Errorf("resolve duplicate group: %w", derr)
	}
	// Audit: emit one log line per deleted file path so future
	// post-mortems can answer "what did housekeeping touch?".
	keptPath := ""
	for _, f := range group.Files {
		if f.ID == group.BestFileID {
			keptPath = f.Path
			break
		}
	}
	for _, f := range group.Files {
		if f.ID == group.BestFileID {
			continue
		}
		e.logf("info", "duplicate-deleted task=%d kind=%s group=%s file_id=%d size=%d path=%q kept_id=%d kept_path=%q",
			t.ID, t.Kind, group.ID, f.ID, f.Size, f.Path, group.BestFileID, keptPath)
	}
	e.logf("info", "duplicate-resolved id=%d kind=%s title=%q deleted=%d reclaimed=%d",
		t.ID, t.Kind, title, deleted, reclaimed)
	return nil
}

// payloadIntPtr extracts an int from a JSON-decoded payload (where
// numeric values arrive as float64) and returns it as *int. Returns
// nil if the key is missing or not numeric.
func payloadIntPtr(p map[string]any, key string) *int {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}
	switch x := v.(type) {
	case float64:
		i := int(x)
		return &i
	case int:
		return &x
	case int64:
		i := int(x)
		return &i
	}
	return nil
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
	dstLib, _ := t.Payload["dst_lib"].(string)

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

	// Pre-walk to compute total byte budget — drives the progress bar in
	// the WebUI. Cheap (stat-only) compared to the upcoming move work.
	var totalBytes int64
	var totalFiles int
	_ = filepath.Walk(src, func(p string, fi os.FileInfo, werr error) error {
		if werr != nil || fi == nil || fi.IsDir() {
			return nil
		}
		totalBytes += fi.Size()
		totalFiles++
		return nil
	})

	prog := e.startTaskOp(t.ID, src, dst, totalFiles, totalBytes)
	defer prog.finish(nil)

	moved := 0
	skipped := 0
	var doneBytes int64
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
				e.updateParseDecisionTargetPath(path, target)
				skipped++
				doneBytes += info.Size()
				prog.fileSkipped(rel, info.Size(), doneBytes, totalBytes)
				return nil
			}
			return fmt.Errorf("size mismatch at %s (src=%d dst=%d) — manual review required",
				target, info.Size(), existing.Size())
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}

		prog.fileStarted(rel, info.Size(), doneBytes, totalBytes)
		fileSize := info.Size()
		res, err := e.transferer.Move(path, target, transfer.TransferOptions{
			Timeout:   30 * time.Minute,
			TargetUID: -1,
			TargetGID: -1,
			Progress: func(cur, _ int64) {
				prog.fileBytes(rel, cur, fileSize, doneBytes+cur, totalBytes)
			},
		})
		if err != nil {
			prog.fileFailed(rel, err)
			return fmt.Errorf("move %s -> %s: %w", path, target, err)
		}
		if res != nil && res.Error != nil {
			prog.fileFailed(rel, res.Error)
			return fmt.Errorf("move %s -> %s: %w", path, target, res.Error)
		}

		if file, gerr := e.db.GetMediaFile(path); gerr == nil && file != nil {
			_ = e.db.DeleteMediaFile(path)
			file.Path = target
			if dstLib != "" {
				file.LibraryRoot = dstLib
			}
			_ = e.db.UpsertMediaFile(file)
		}
		e.updateParseDecisionTargetPath(path, target)

		moved++
		doneBytes += fileSize
		prog.fileCompleted(rel, fileSize, doneBytes, totalBytes)
		return nil
	})

	if walkErr != nil {
		prog.finish(walkErr)
		return walkErr
	}

	if err := removeIfEmptyTree(src); err != nil {
		e.logf("warn", "merge: source not fully empty after move src=%s err=%v", src, err)
	}

	e.logf("info", "merged %d file(s), skipped %d duplicate(s) src=%s dst=%s",
		moved, skipped, src, dst)
	prog.summary(moved, skipped, doneBytes, totalBytes)
	return nil
}

func (e *Engine) updateParseDecisionTargetPath(oldPath, newPath string) {
	if e == nil || e.db == nil || oldPath == "" || newPath == "" || oldPath == newPath {
		return
	}
	now := time.Now().UTC()
	if _, err := e.db.DB().Exec(`
		UPDATE parse_decisions
		   SET target_path = ?, target_at = ?
		 WHERE target_path = ?
		   AND organize_outcome = 'success'`, newPath, now, oldPath); err != nil {
		e.logf("warn", "update parse decision target failed old=%s new=%s err=%v", oldPath, newPath, err)
	}
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
