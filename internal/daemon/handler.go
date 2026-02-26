package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"github.com/Nomadcxx/jellywatch/internal/notify"
	"github.com/Nomadcxx/jellywatch/internal/organizer"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
	"github.com/Nomadcxx/jellywatch/internal/transfer"
	"github.com/Nomadcxx/jellywatch/internal/watcher"
)

type MediaHandler struct {
	tvOrganizer     *organizer.Organizer // NEW: TV-specific organizer
	movieOrganizer  *organizer.Organizer // NEW: Movie-specific organizer
	notifyManager   *notify.Manager
	tvLibraries     []string
	movieLibs       []string
	tvWatchPaths    []string // TV watch folders for source hint
	movieWatchPaths []string // Movie watch folders for source hint
	debounceTime    time.Duration
	pending         map[string]*time.Timer
	mu              sync.Mutex
	dryRun          bool
	stats           *Stats
	logger          *logging.Logger
	sonarrClient    *sonarr.Client
	db              *database.MediaDB
	activityLogger  *activity.Logger
	playbackLocks   *jellyfin.PlaybackLockManager
	deferredQueue   *jellyfin.DeferredQueue
}

type Stats struct {
	mu               sync.RWMutex
	MoviesProcessed  int64
	TVProcessed      int64
	BytesTransferred int64
	Errors           int64
	LastProcessed    time.Time
	StartTime        time.Time
}

func NewStats() *Stats {
	return &Stats{
		StartTime: time.Now(),
	}
}

func (s *Stats) RecordMovie(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MoviesProcessed++
	s.BytesTransferred += bytes
	s.LastProcessed = time.Now()
}

func (s *Stats) RecordTV(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TVProcessed++
	s.BytesTransferred += bytes
	s.LastProcessed = time.Now()
}

func (s *Stats) RecordError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Errors++
}

func (s *Stats) Snapshot() StatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StatsSnapshot{
		MoviesProcessed:  s.MoviesProcessed,
		TVProcessed:      s.TVProcessed,
		BytesTransferred: s.BytesTransferred,
		Errors:           s.Errors,
		LastProcessed:    s.LastProcessed,
		Uptime:           time.Since(s.StartTime),
	}
}

type StatsSnapshot struct {
	MoviesProcessed  int64
	TVProcessed      int64
	BytesTransferred int64
	Errors           int64
	LastProcessed    time.Time
	Uptime           time.Duration
}

type MediaHandlerConfig struct {
	TVLibraries     []string
	MovieLibs       []string
	TVWatchPaths    []string // New
	MovieWatchPaths []string // New
	DebounceTime    time.Duration
	DryRun          bool
	Timeout         time.Duration
	Backend         transfer.Backend
	NotifyManager   *notify.Manager
	Logger          *logging.Logger
	TargetUID       int
	TargetGID       int
	FileMode        os.FileMode
	DirMode         os.FileMode
	SonarrClient    *sonarr.Client
	JellyfinClient  *jellyfin.Client
	PlaybackSafety  bool
	Database        *database.MediaDB
	ConfigDir       string
	PlaybackLocks   *jellyfin.PlaybackLockManager
	DeferredQueue   *jellyfin.DeferredQueue
}

func NewMediaHandler(cfg MediaHandlerConfig) (*MediaHandler, error) {
	if cfg.DebounceTime == 0 {
		cfg.DebounceTime = 10 * time.Second
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.Logger == nil {
		cfg.Logger = logging.Nop()
	}

	var activityLogger *activity.Logger
	if cfg.ConfigDir != "" {
		var err error
		activityLogger, err = activity.NewLogger(cfg.ConfigDir)
		if err != nil {
			cfg.Logger.Warn("handler", "Failed to create activity logger", logging.F("error", err.Error()))
		} else {
			cfg.Logger.Info("handler", "Activity logger initialized", logging.F("log_dir", activityLogger.GetLogDir()))
		}
	}

	// Create TV-specific organizer
	tvOrgOpts := []func(*organizer.Organizer){
		organizer.WithDryRun(cfg.DryRun),
		organizer.WithTimeout(cfg.Timeout),
		organizer.WithBackend(cfg.Backend),
		organizer.WithPlaybackLockManager(cfg.PlaybackLocks),
		organizer.WithDeferredQueue(cfg.DeferredQueue),
	}
	if cfg.SonarrClient != nil {
		tvOrgOpts = append(tvOrgOpts, organizer.WithSonarrClient(cfg.SonarrClient))
	}
	if cfg.JellyfinClient != nil {
		tvOrgOpts = append(tvOrgOpts, organizer.WithJellyfinClient(cfg.JellyfinClient, cfg.PlaybackSafety))
	}
	if cfg.TargetUID >= 0 || cfg.TargetGID >= 0 || cfg.FileMode != 0 || cfg.DirMode != 0 {
		tvOrgOpts = append(tvOrgOpts, organizer.WithPermissions(cfg.TargetUID, cfg.TargetGID, cfg.FileMode, cfg.DirMode))
	}
	tvOrganizer, err := organizer.NewOrganizer(cfg.TVLibraries, tvOrgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create TV organizer: %w", err)
	}

	// Create Movie-specific organizer
	movieOrgOpts := []func(*organizer.Organizer){
		organizer.WithDryRun(cfg.DryRun),
		organizer.WithTimeout(cfg.Timeout),
		organizer.WithBackend(cfg.Backend),
		organizer.WithPlaybackLockManager(cfg.PlaybackLocks),
		organizer.WithDeferredQueue(cfg.DeferredQueue),
	}
	if cfg.JellyfinClient != nil {
		movieOrgOpts = append(movieOrgOpts, organizer.WithJellyfinClient(cfg.JellyfinClient, cfg.PlaybackSafety))
	}
	if cfg.TargetUID >= 0 || cfg.TargetGID >= 0 || cfg.FileMode != 0 || cfg.DirMode != 0 {
		movieOrgOpts = append(movieOrgOpts, organizer.WithPermissions(cfg.TargetUID, cfg.TargetGID, cfg.FileMode, cfg.DirMode))
	}
	movieOrganizer, err := organizer.NewOrganizer(cfg.MovieLibs, movieOrgOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Movie organizer: %w", err)
	}

	return &MediaHandler{
		tvOrganizer:     tvOrganizer,
		movieOrganizer:  movieOrganizer,
		notifyManager:   cfg.NotifyManager,
		tvLibraries:     cfg.TVLibraries,
		movieLibs:       cfg.MovieLibs,
		tvWatchPaths:    cfg.TVWatchPaths,
		movieWatchPaths: cfg.MovieWatchPaths,
		debounceTime:    cfg.DebounceTime,
		pending:         make(map[string]*time.Timer),
		dryRun:          cfg.DryRun,
		stats:           NewStats(),
		logger:          cfg.Logger,
		sonarrClient:    cfg.SonarrClient,
		db:              cfg.Database,
		activityLogger:  activityLogger,
		playbackLocks:   cfg.PlaybackLocks,
		deferredQueue:   cfg.DeferredQueue,
	}, nil
}

func (h *MediaHandler) HandleFileEvent(event watcher.FileEvent) error {
	if event.Type == watcher.EventDelete {
		return nil
	}

	if !h.IsMediaFile(event.Path) {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if timer, exists := h.pending[event.Path]; exists {
		timer.Stop()
		delete(h.pending, event.Path)
	}

	h.pending[event.Path] = time.AfterFunc(h.debounceTime, func() {
		h.processFile(event.Path)
	})

	return nil
}

func (h *MediaHandler) logEntry(
	result *organizer.OrganizationResult,
	mediaType notify.MediaType,
	parsedTitle string,
	parsedYear *int,
	parseMethod activity.ParseMethod,
	aiConfidence float64,
	duration time.Duration,
	sonarrNotified bool,
	radarrNotified bool,
) {
	if h.activityLogger == nil {
		return
	}

	var aiConf *float64
	if aiConfidence > 0 {
		aiConf = &aiConfidence
	}

	var target string
	if result.TargetPath != "" {
		target = result.TargetPath
	}

	entry := activity.Entry{
		Action:         "organize",
		Source:         result.SourcePath,
		Target:         target,
		MediaType:      mediaType.String(),
		ParseMethod:    parseMethod,
		ParsedTitle:    parsedTitle,
		ParsedYear:     parsedYear,
		AIConfidence:   aiConf,
		Success:        result.Success,
		Bytes:          result.BytesCopied,
		DurationMs:     duration.Milliseconds(),
		SonarrNotified: sonarrNotified,
		RadarrNotified: radarrNotified,
	}

	if !result.Success && result.Error != nil {
		entry.Error = result.Error.Error()
	}

	if err := h.activityLogger.Log(entry); err != nil {
		h.logger.Warn("handler", "Failed to log activity", logging.F("error", err.Error()))
	}
}

// getSourceHint determines if a path is under a configured TV or Movie watch folder
func (h *MediaHandler) getSourceHint(path string) naming.SourceHint {
	// Check TV watch folders
	for _, tvPath := range h.tvWatchPaths {
		if strings.HasPrefix(path, tvPath) {
			return naming.SourceTV
		}
	}

	// Check Movie watch folders
	for _, moviePath := range h.movieWatchPaths {
		if strings.HasPrefix(path, moviePath) {
			return naming.SourceMovie
		}
	}

	return naming.SourceUnknown
}

func (h *MediaHandler) processFile(path string) {
	startTime := time.Now()

	h.mu.Lock()
	delete(h.pending, path)
	h.mu.Unlock()

	filename := filepath.Base(path)
	h.logger.Info("handler", "Processing file", logging.F("filename", filename), logging.F("path", path))

	if h.dryRun {
		h.logger.Info("handler", "Dry run - would process", logging.F("filename", filename))
		return
	}

	var result *organizer.OrganizationResult
	var err error
	var targetLib string
	var mediaType notify.MediaType

	var parsedTitle string
	var parsedYear *int
	parseMethod := activity.MethodRegex
	aiConfidence := 0.0

	isObfuscated := naming.IsObfuscatedFilename(filename)
	if isObfuscated {
		h.logger.Info("handler", "Detected obfuscated filename, using folder name", logging.F("filename", filename))
	}

	sourceHint := h.getSourceHint(path)
	isTVEpisode := naming.IsTVEpisodeFromPath(path, sourceHint)

	if isTVEpisode {
		if len(h.tvLibraries) == 0 {
			h.logger.Warn("handler", "No TV libraries configured, skipping", logging.F("filename", filename))
			return
		}
		mediaType = notify.MediaTypeTVEpisode

		if tvInfo, err := naming.ParseTVShowName(path); err == nil {
			parsedTitle = tvInfo.Title
			if tvInfo.Year != "" {
				year := 0
				if _, err := fmt.Sscanf(tvInfo.Year, "%d", &year); err == nil {
					parsedYear = &year
				}
			}
		}

		// Use auto-selection (queries Sonarr + filesystem)
		result, err = h.tvOrganizer.OrganizeTVEpisodeAuto(path, func(p string) (int64, error) {
			info, err := os.Stat(p)
			if err != nil {
				return 0, err
			}
			return info.Size(), nil
		})

		// Extract target library from result for health check logging
		if result != nil && result.TargetPath != "" {
			targetLib = filepath.Dir(filepath.Dir(filepath.Dir(result.TargetPath)))
		}
	} else {
		if len(h.movieLibs) == 0 {
			h.logger.Warn("handler", "No movie libraries configured, skipping", logging.F("filename", filename))
			return
		}
		targetLib = h.movieLibs[0]
		mediaType = notify.MediaTypeMovie

		if movieInfo, err := naming.ParseMovieName(path); err == nil {
			parsedTitle = movieInfo.Title
			if movieInfo.Year != "" {
				year := 0
				if _, err := fmt.Sscanf(movieInfo.Year, "%d", &year); err == nil {
					parsedYear = &year
				}
			}
		}

		if !h.checkTargetHealth(targetLib) {
			h.logger.Warn("handler", "Target library unhealthy, skipping", logging.F("filename", filename), logging.F("target", targetLib))
			return
		}

		result, err = h.movieOrganizer.OrganizeMovie(path, targetLib)
	}

	duration := time.Since(startTime)

	// Track notification results
	sonarrNotified := false
	radarrNotified := false

	if err != nil {
		h.logger.Error("handler", "Organization failed", err, logging.F("filename", filename))
		h.stats.RecordError()
		h.logEntry(result, mediaType, parsedTitle, parsedYear, parseMethod, aiConfidence, duration, sonarrNotified, radarrNotified)
		return
	}

	if result.Success {
		h.logger.Info("handler", "Organized successfully",
			logging.F("source", filepath.Base(result.SourcePath)),
			logging.F("target", result.TargetPath),
			logging.F("size_mb", float64(result.BytesCopied)/(1024*1024)),
			logging.F("duration", result.Duration.String()))

		if mediaType == notify.MediaTypeMovie {
			h.stats.RecordMovie(result.BytesCopied)
		} else {
			h.stats.RecordTV(result.BytesCopied)
		}

		// Send notifications and track results
		sonarrNotified, radarrNotified = h.sendNotificationsWithTracking(result, mediaType)
	} else if result.Skipped {
		h.logger.Info("handler", "Skipped", logging.F("filename", filename), logging.F("reason", result.SkipReason))
	} else {
		h.logger.Error("handler", "Organization failed", result.Error, logging.F("filename", filename))
		h.stats.RecordError()
	}

	// Log activity entry after notifications
	h.logEntry(result, mediaType, parsedTitle, parsedYear, parseMethod, aiConfidence, duration, sonarrNotified, radarrNotified)
}

func (h *MediaHandler) sendNotificationsWithTracking(result *organizer.OrganizationResult, mediaType notify.MediaType) (sonarrNotified, radarrNotified bool) {
	if h.notifyManager == nil {
		return false, false
	}

	event := notify.OrganizationEvent{
		MediaType:   mediaType,
		SourcePath:  result.SourcePath,
		TargetPath:  result.TargetPath,
		TargetDir:   filepath.Dir(result.TargetPath),
		BytesCopied: result.BytesCopied,
		Duration:    result.Duration,
	}

	h.notifyManager.Notify(event)

	// Track which notifiers would have been called based on media type
	// Sonarr handles TV episodes, Radarr handles movies
	if mediaType == notify.MediaTypeTVEpisode {
		sonarrNotified = true
	} else if mediaType == notify.MediaTypeMovie {
		radarrNotified = true
	}

	return sonarrNotified, radarrNotified
}

func (h *MediaHandler) checkTargetHealth(targetLib string) bool {
	err := transfer.CheckDiskHealthForTransfer("", targetLib, 5*time.Second, 0)
	if err != nil {
		h.logger.Warn("handler", "Health check failed", logging.F("target", targetLib), logging.F("error", err.Error()))
		return false
	}
	return true
}

func (h *MediaHandler) IsMediaFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	mediaExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
		".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
	}
	return mediaExts[ext]
}

func (h *MediaHandler) Stats() StatsSnapshot {
	return h.stats.Snapshot()
}

func (h *MediaHandler) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for path, timer := range h.pending {
		timer.Stop()
		delete(h.pending, path)
	}

	if h.notifyManager != nil {
		h.notifyManager.Close()
	}

	if h.activityLogger != nil {
		h.activityLogger.Close()
	}
}

func (h *MediaHandler) PlaybackLockManager() *jellyfin.PlaybackLockManager {
	return h.playbackLocks
}

func (h *MediaHandler) DeferredQueue() *jellyfin.DeferredQueue {
	return h.deferredQueue
}

// HandleJellyfinWebhookEvent mutates playback state from webhook events.
func (h *MediaHandler) HandleJellyfinWebhookEvent(event jellyfin.WebhookEvent) {
	path := strings.TrimSpace(event.ItemPath)
	switch event.NotificationType {
	case jellyfin.EventPlaybackStart:
		if path == "" || h.playbackLocks == nil {
			return
		}
		h.playbackLocks.Lock(path, jellyfin.PlaybackInfo{
			UserName:   event.UserName,
			DeviceName: event.DeviceName,
			ClientName: event.ClientName,
			ItemID:     event.ItemID,
			StartedAt:  time.Now(),
		})
		if h.logger != nil {
			h.logger.Info("handler", "Playback lock added", logging.F("path", path), logging.F("user", event.UserName))
		}
	case jellyfin.EventPlaybackStop:
		if path == "" {
			return
		}
		if h.playbackLocks != nil {
			h.playbackLocks.Unlock(path)
		}
		h.replayDeferredOperationsForPath(path)
		if h.logger != nil {
			h.logger.Info("handler", "Playback lock removed", logging.F("path", path))
		}
	case jellyfin.EventItemAdded:
		itemID := strings.TrimSpace(event.ItemID)
		if h.db != nil && path != "" && itemID != "" {
			if err := h.db.UpsertJellyfinItem(path, itemID, event.ItemName, event.ItemType); err != nil {
				if h.logger != nil {
					h.logger.Warn("handler", "Failed to upsert Jellyfin item", logging.F("path", path), logging.F("item_id", itemID), logging.F("error", err.Error()))
				}
				return
			}
		}
		if h.logger != nil {
			h.logger.Info("handler", "Jellyfin item added", logging.F("path", path), logging.F("item_id", itemID), logging.F("name", event.ItemName), logging.F("type", event.ItemType))
		}
	case jellyfin.EventTaskCompleted:
		if h.logger != nil {
			h.logger.Info("handler", "Jellyfin task completed", logging.F("task", event.TaskName), logging.F("item", event.ItemName))
		}
		h.logJellyfinActivity("jellyfin_task_completed", event.TaskName, event.ItemName, true, "")
		if h.db != nil && h.activityLogger != nil {
			h.runJellyfinVerificationPass()
		}
	case jellyfin.EventLibraryChanged:
		if h.logger != nil {
			h.logger.Info("handler", "Jellyfin library changed", logging.F("server", event.ServerName), logging.F("item", event.ItemName))
		}
		h.logJellyfinActivity("jellyfin_library_changed", event.ServerName, event.ItemName, true, "")
	}
}

func (h *MediaHandler) logJellyfinActivity(action, source, target string, success bool, errMsg string) {
	if h.activityLogger == nil {
		return
	}

	entry := activity.Entry{
		Action:    action,
		Source:    source,
		Target:    target,
		MediaType: "jellyfin",
		Success:   success,
	}
	if errMsg != "" {
		entry.Error = errMsg
	}
	if err := h.activityLogger.Log(entry); err != nil && h.logger != nil {
		h.logger.Warn("handler", "Failed to log Jellyfin activity", logging.F("error", err.Error()))
	}
}

func (h *MediaHandler) runJellyfinVerificationPass() {
	if h == nil || h.db == nil || h.activityLogger == nil {
		return
	}

	entries, err := h.activityLogger.GetRecentEntries(200)
	if err != nil {
		h.logJellyfinActivity("jellyfin_verification_summary", "read_activity", "", false, err.Error())
		return
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	checked := 0
	mismatches := 0

	for _, entry := range entries {
		if entry.Action != "organize" || !entry.Success || strings.TrimSpace(entry.Target) == "" {
			continue
		}
		if !entry.Timestamp.IsZero() && entry.Timestamp.Before(cutoff) {
			continue
		}

		checked++
		item, err := h.db.GetJellyfinItemByPath(entry.Target)
		if err != nil || item == nil {
			mismatches++
			h.logJellyfinActivity("jellyfin_verification_mismatch", entry.Target, entry.ParsedTitle, false, "path not confirmed in jellyfin")
			continue
		}
	}

	h.logJellyfinActivity(
		"jellyfin_verification_summary",
		fmt.Sprintf("checked=%d", checked),
		fmt.Sprintf("mismatches=%d", mismatches),
		mismatches == 0,
		"",
	)
}

func (h *MediaHandler) replayDeferredOperationsForPath(path string) {
	if h.deferredQueue == nil {
		return
	}
	ops := h.deferredQueue.RemoveForPath(path)
	for _, op := range ops {
		h.replayDeferredOperation(op)
	}
}

func (h *MediaHandler) replayDeferredOperation(op jellyfin.DeferredOp) {
	if strings.TrimSpace(op.SourcePath) == "" {
		return
	}
	// Replay requires a fully initialized handler pipeline.
	if h.logger == nil || h.tvOrganizer == nil || h.movieOrganizer == nil {
		return
	}
	if h.logger != nil {
		h.logger.Info("handler", "Replaying deferred operation",
			logging.F("type", op.Type),
			logging.F("source", op.SourcePath),
			logging.F("target", op.TargetPath))
	}
	// Re-run through the regular process pipeline so classification and notifications stay consistent.
	go h.processFile(op.SourcePath)
}

func (h *MediaHandler) PruneActivityLogs(days int) error {
	if h.activityLogger == nil {
		return nil
	}
	return h.activityLogger.PruneOld(days)
}
