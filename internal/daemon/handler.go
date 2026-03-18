package daemon

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
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
	tvOrganizer      *organizer.Organizer // NEW: TV-specific organizer
	movieOrganizer   *organizer.Organizer // NEW: Movie-specific organizer
	notifyManager    *notify.Manager
	tvLibraries      []string
	movieLibs        []string
	tvWatchPaths     []string // TV watch folders for source hint
	movieWatchPaths  []string // Movie watch folders for source hint
	debounceTime     time.Duration
	pending          map[string]*time.Timer
	transientRetries map[string]int
	mu               sync.Mutex
	dryRun           bool
	stats            *Stats
	logger           *logging.Logger
	sonarrClient     *sonarr.Client
	db               *database.MediaDB
	activityLogger   *activity.Logger
	playbackLocks    *jellyfin.PlaybackLockManager
	deferredQueue    *jellyfin.DeferredQueue
	pendingAI        map[string]*PendingItem
	pendingAICap     int
	aiMatcher        *ai.Matcher
	aiConfig         config.AIConfig
	aiRateLimiter    *AIRateLimiter
	enhanceLogger    *EnhanceLogger
	aiEnabled        bool
}

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

var transientRetryDelay = 2 * time.Second

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
	AIEnabled       bool
	AIMatcher       *ai.Matcher
	AIConfig        config.AIConfig
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

	var enhanceLog *EnhanceLogger
	var rateLimiter *AIRateLimiter
	if cfg.AIEnabled && cfg.ConfigDir != "" {
		enhanceLog = NewEnhanceLogger(cfg.ConfigDir)
		rateLimiter = NewAIRateLimiter(cfg.AIConfig.HourlyLimit, cfg.AIConfig.DailyLimit)
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
		tvOrganizer:      tvOrganizer,
		movieOrganizer:   movieOrganizer,
		notifyManager:    cfg.NotifyManager,
		tvLibraries:      cfg.TVLibraries,
		movieLibs:        cfg.MovieLibs,
		tvWatchPaths:     cfg.TVWatchPaths,
		movieWatchPaths:  cfg.MovieWatchPaths,
		debounceTime:     cfg.DebounceTime,
		pending:          make(map[string]*time.Timer),
		transientRetries: make(map[string]int),
		dryRun:           cfg.DryRun,
		stats:            NewStats(),
		logger:           cfg.Logger,
		sonarrClient:     cfg.SonarrClient,
		db:               cfg.Database,
		activityLogger:   activityLogger,
		playbackLocks:    cfg.PlaybackLocks,
		deferredQueue:    cfg.DeferredQueue,
		pendingAI:        make(map[string]*PendingItem),
		pendingAICap:     100,
		aiMatcher:        cfg.AIMatcher,
		aiConfig:         cfg.AIConfig,
		aiRateLimiter:    rateLimiter,
		enhanceLogger:    enhanceLog,
		aiEnabled:        cfg.AIEnabled,
	}, nil
}

func (h *MediaHandler) allWatchPaths() []string {
	paths := make([]string, 0, len(h.tvWatchPaths)+len(h.movieWatchPaths))
	paths = append(paths, h.tvWatchPaths...)
	paths = append(paths, h.movieWatchPaths...)
	return paths
}

func (h *MediaHandler) HandleFileEvent(event watcher.FileEvent) error {
	if event.Type == watcher.EventDelete {
		return nil
	}

	normalizedPath, reason, action := normalizeEventPath(event.Path, h.allWatchPaths())
	if action == ingestReject {
		h.logger.Warn("handler", "Rejected event path", logging.F("path", event.Path), logging.F("reason", reason))
		return nil
	}
	if !h.IsMediaFile(normalizedPath) {
		return nil
	}

	if action == ingestDefer {
		h.mu.Lock()
		defer h.mu.Unlock()

		if h.transientRetries[normalizedPath] >= 1 {
			h.logger.Warn("handler", "Skipping repeated transient path retry",
				logging.F("path", normalizedPath),
				logging.F("reason", reason))
			return nil
		}

		if timer, exists := h.pending[normalizedPath]; exists {
			timer.Stop()
			delete(h.pending, normalizedPath)
		}

		h.transientRetries[normalizedPath]++
		h.pending[normalizedPath] = time.AfterFunc(transientRetryDelay, func() {
			h.processFile(normalizedPath)
		})

		h.logger.Info("handler", "Deferred transient path",
			logging.F("path", normalizedPath),
			logging.F("reason", reason),
			logging.F("delay", transientRetryDelay.String()))
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if timer, exists := h.pending[normalizedPath]; exists {
		timer.Stop()
		delete(h.pending, normalizedPath)
	}

	h.pending[normalizedPath] = time.AfterFunc(h.debounceTime, func() {
		h.processFile(normalizedPath)
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
	// Skip files still inside Sabnzbd's _UNPACK_ staging folder.
	// After extraction, Sabnzbd renames the folder and the watcher/scanner picks up the real path.
	if strings.Contains(path, string(os.PathSeparator)+"_UNPACK_") {
		h.logger.Info("handler", "Skipping _UNPACK_ path — extraction still in progress",
			logging.F("path", path))
		h.mu.Lock()
		delete(h.pending, path)
		delete(h.transientRetries, path)
		h.mu.Unlock()
		return
	}

	startTime := time.Now()

	h.mu.Lock()
	delete(h.pending, path)
	delete(h.transientRetries, path)
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

		tvInfo, parseErr := naming.ParseTVShowFromPath(path)
		if parseErr == nil {
			parsedTitle = tvInfo.Title
			if tvInfo.Year != "" {
				year := 0
				if _, err := fmt.Sscanf(tvInfo.Year, "%d", &year); err == nil {
					parsedYear = &year
				}
			}

			confidence := naming.CalculateTitleConfidence(tvInfo.Title, filename)
			if h.aiEnabled && confidence < h.aiConfig.AutoTriggerThreshold {
				h.queueForAI(path, filename, tvInfo, nil, "tv", confidence, "")
				return
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
		h.cleanupSourceDir(path)
	} else if result.Skipped {
		h.logger.Info("handler", "Skipped", logging.F("filename", filename), logging.F("reason", result.SkipReason))
	} else {
		h.logger.Error("handler", "Organization failed", result.Error, logging.F("filename", filename))
		h.stats.RecordError()
	}

	// Log activity entry after notifications
	h.logEntry(result, mediaType, parsedTitle, parsedYear, parseMethod, aiConfidence, duration, sonarrNotified, radarrNotified)
}

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
		return
	}

	h.pendingAI[path] = &PendingItem{
		Path:       path,
		Filename:   filename,
		TVInfo:     tvInfo,
		MovieInfo:  movieInfo,
		MediaType:  mediaType,
		Confidence: confidence,
		QueuedAt:   time.Now(),
		TargetLib:  targetLib,
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

// containsVideoFilesRecursive reports whether dir or any of its descendants
// contains at least one file recognised by IsMediaFile.
// Short-circuits on the first match via fs.SkipAll.
func (h *MediaHandler) containsVideoFilesRecursive(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && h.IsMediaFile(path) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// cleanupSourceDir removes the source download directory after a successful move,
// walking up parent directories until it reaches a watch root.
//
// Gate: if sourcePath still exists the source was preserved (keepSource, dry-run),
// so cleanup is skipped entirely.
//
// At each level it checks whether any video files remain; if none do, the whole
// directory tree at that level is removed via os.RemoveAll (clearing junk files,
// SABnzbd markers, empty subdirs, etc.). Concurrent RemoveAll on the same path is
// safe — os.RemoveAll returns nil for non-existent paths.
func (h *MediaHandler) cleanupSourceDir(sourcePath string) {
	// Gate: source still present means keepSource or dry-run — do nothing.
	if _, err := os.Stat(sourcePath); err == nil {
		return
	}

	// Build a set of watch roots for O(1) boundary checks.
	// filepath.Clean strips trailing slashes, matching normalizeEventPath behaviour.
	rootSet := make(map[string]struct{})
	for _, p := range h.tvWatchPaths {
		rootSet[filepath.Clean(p)] = struct{}{}
	}
	for _, p := range h.movieWatchPaths {
		rootSet[filepath.Clean(p)] = struct{}{}
	}

	dir := filepath.Dir(sourcePath)
	for {
		clean := filepath.Clean(dir)
		if _, isRoot := rootSet[clean]; isRoot {
			return // never remove a watch root
		}

		if h.containsVideoFilesRecursive(dir) {
			return // more episodes pending — leave directory alone
		}

		if err := os.RemoveAll(dir); err != nil {
			h.logger.Warn("handler", "Failed to remove source directory",
				logging.F("dir", dir), logging.F("error", err.Error()))
			// Continue walking up — a permission error on one level
			// should not prevent cleanup of parent directories.
		} else {
			h.logger.Info("handler", "Cleaned up source directory", logging.F("dir", dir))
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return // filesystem root reached — stop
		}
		dir = parent
	}
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

		// No AI matcher available
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
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			aiYear = fmt.Sprintf("%d", *aiResult.Year.Int())
		}

		classification := ClassifyChange(regexTitle, aiResult.Title, regexYear, aiYear, item.MediaType, aiResult.Type)

		if classification.Safe && aiResult.Confidence >= classification.MinConfidence {
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
		tvInfo := naming.TVShowInfo{
			Title: aiResult.Title,
		}
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			tvInfo.Year = fmt.Sprintf("%d", *aiResult.Year.Int())
		}
		if aiResult.Season != nil && aiResult.Season.Int() != nil {
			tvInfo.Season = *aiResult.Season.Int()
		} else if item.TVInfo != nil {
			tvInfo.Season = item.TVInfo.Season
		}
		if len(aiResult.Episodes) > 0 {
			tvInfo.Episode = aiResult.Episodes[0]
		} else if item.TVInfo != nil {
			tvInfo.Episode = item.TVInfo.Episode
		}

		// Determine target library (TV auto-selection uses first TV library)
		targetLib := ""
		if len(h.tvLibraries) > 0 {
			targetLib = h.tvLibraries[0]
		}

		result, err = h.tvOrganizer.OrganizeTVWithParsed(item.Path, targetLib, tvInfo)
	} else {
		movieInfo := naming.MovieInfo{
			Title: aiResult.Title,
		}
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			movieInfo.Year = fmt.Sprintf("%d", *aiResult.Year.Int())
		}

		result, err = h.movieOrganizer.OrganizeMovieWithParsed(item.Path, item.TargetLib, movieInfo)
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
		h.cleanupSourceDir(item.Path)
	}
}

func (h *MediaHandler) PruneActivityLogs(days int) error {
	if h.activityLogger == nil {
		return nil
	}
	return h.activityLogger.PruneOld(days)
}
