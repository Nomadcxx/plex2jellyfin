package daemon

import (
	"context"
	"encoding/json"
	"errors"
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
	transientWarned  map[string]bool
	mu               sync.Mutex
	dryRun           bool
	stats            *Stats
	logger           *logging.Logger
	sonarrClient     *sonarr.Client
	db               *database.MediaDB
	activityLogger   *activity.Logger
	playbackLocks    *jellyfin.PlaybackLockManager
	deferredQueue    *jellyfin.DeferredQueue
	pathTranslator   *jellyfin.PathTranslator
	pendingAI        map[string]*PendingItem
	pendingAICap     int
	aiMatcher        *ai.Matcher
	aiCache          *ai.Cache
	aiConfig         config.AIConfig
	aiRateLimiter    *AIRateLimiter
	enhanceLogger    *EnhanceLogger
	aiEnabled        bool
	// loggedErrors dedupes repeat ERROR emissions for the same (path, error)
	// pair across retry scans within a process lifetime. Cleared only on
	// restart — intentional: first retry after daemon restart re-logs once.
	loggedErrors map[string]struct{}
	// lastAIPermanentKey dedupes the AI-fallback WARN when the same permanent
	// error (e.g. Ollama 403 subscription) repeats. First occurrence logs
	// WARN; subsequent identical errors log at INFO until the key changes.
	lastAIPermanentKey string
	// aiBudgetWarned tracks whether an 80%-budget warning has already been
	// emitted for the current hour/day window. Reset when the window rolls.
	aiBudgetWarnedHour bool
	aiBudgetWarnedDay  bool
	// targetHealthState remembers the last known healthy/unhealthy state per
	// target library so we only log on transitions, not every check.
	targetHealthState map[string]bool
	// unparseableCache defers re-processing of files that fail with
	// deterministic, non-recoverable parse/organize errors so the periodic
	// scanner doesn't burn cycles re-attempting them every 5 minutes.
	unparseableCache *NegativeCache
}

type PendingItem struct {
	Path            string
	Filename        string
	TVInfo          *naming.TVShowInfo
	MovieInfo       *naming.MovieInfo
	MediaType       string
	Confidence      float64
	QueuedAt        time.Time
	TargetLib       string
	AttemptCount    int
	LastAttemptAt   time.Time
	Blacklisted     bool
	ParseDecisionID int64
}

const maxAIRetryAttempts = 10

// aiRetryBackoff returns the backoff duration for a given attempt count.
// Schedule: 30s, 60s, 2m, 5m, 15m, 30m, then 30m for all subsequent.
func aiRetryBackoff(attempt int) time.Duration {
	backoffs := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		2 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
	}
	if attempt <= 0 {
		return backoffs[0]
	}
	if attempt > len(backoffs) {
		return backoffs[len(backoffs)-1]
	}
	return backoffs[attempt-1]
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
	PathTranslator  *jellyfin.PathTranslator
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

	// Wire an AI cache into the daemon path so duplicate filenames during
	// bulk imports don't each cost an Ollama round-trip. Previously only
	// the scanner's AIHelper used the cache; daemon events always hit
	// upstream.
	var aiCache *ai.Cache
	if cfg.AIEnabled && cfg.AIConfig.CacheEnabled && cfg.Database != nil {
		aiCache = ai.NewCache(cfg.Database.DB())
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
		transientWarned:  make(map[string]bool),
		dryRun:           cfg.DryRun,
		stats:            NewStats(),
		logger:           cfg.Logger,
		sonarrClient:     cfg.SonarrClient,
		db:               cfg.Database,
		activityLogger:   activityLogger,
		playbackLocks:    cfg.PlaybackLocks,
		deferredQueue:    cfg.DeferredQueue,
		pathTranslator:   cfg.PathTranslator,
		pendingAI:        make(map[string]*PendingItem),
		pendingAICap:     100,
		aiMatcher:        cfg.AIMatcher,
		aiCache:          aiCache,
		aiConfig:         cfg.AIConfig,
		aiRateLimiter:    rateLimiter,
		enhanceLogger:    enhanceLog,
		aiEnabled:         cfg.AIEnabled,
		loggedErrors:      make(map[string]struct{}),
		targetHealthState: make(map[string]bool),
		unparseableCache:  NewNegativeCache(),
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

	// Skip files that are already inside a library directory. This guards
	// against misconfigured watch paths that overlap with library paths,
	// which would otherwise cause every library file to be re-processed.
	if h.isInsideLibrary(normalizedPath) {
		return nil
	}

	if action == ingestDefer {
		h.mu.Lock()
		defer h.mu.Unlock()

		if h.transientRetries[normalizedPath] >= 1 {
			// First skip → WARN once; subsequent skips → DEBUG so a SABnzbd
			// unpack producing dozens of fsnotify events for the same
			// in-progress file doesn't flood the log. We use a separate
			// "warned-once" set rather than incrementing transientRetries,
			// because the counter has different semantics (it caps at 1
			// to bound the work the handler does, not the logging).
			if !h.transientWarned[normalizedPath] {
				h.logger.Warn("handler", "Skipping repeated transient path retry",
					logging.F("path", normalizedPath),
					logging.F("reason", reason))
				h.transientWarned[normalizedPath] = true
			} else {
				h.logger.Debug("handler", "Skipping repeated transient path retry",
					logging.F("path", normalizedPath),
					logging.F("reason", reason))
			}
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
		if errors.Is(result.Error, naming.ErrParseFailed) {
			entry.Deterministic = true
			if result.SourcePath != "" {
				if st, serr := os.Stat(result.SourcePath); serr == nil {
					entry.SourceMtime = st.ModTime().Unix()
				}
			}
		}
	}

	if err := h.activityLogger.Log(entry); err != nil {
		h.logger.Warn("handler", "Failed to log activity", logging.F("error", err.Error()))
	}
}

// emitAIBudgetWarnings fires WARN once per window when hourly or daily AI
// budget crosses 80% of cap, and resets the latch when usage drops back
// below the threshold (e.g. window rollover). Avoids per-call log spam while
// still surfacing budget pressure to operators.
func (h *MediaHandler) emitAIBudgetWarnings(hUsed, hCap, dUsed, dCap int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if hCap > 0 {
		ratio := float64(hUsed) / float64(hCap)
		if ratio >= 0.8 && !h.aiBudgetWarnedHour {
			h.logger.Warn("handler", "AI hourly budget ≥80%",
				logging.F("used", hUsed),
				logging.F("cap", hCap))
			h.aiBudgetWarnedHour = true
		} else if ratio < 0.8 && h.aiBudgetWarnedHour {
			h.aiBudgetWarnedHour = false
		}
	}
	if dCap > 0 {
		ratio := float64(dUsed) / float64(dCap)
		if ratio >= 0.8 && !h.aiBudgetWarnedDay {
			h.logger.Warn("handler", "AI daily budget ≥80%",
				logging.F("used", dUsed),
				logging.F("cap", dCap))
			h.aiBudgetWarnedDay = true
		} else if ratio < 0.8 && h.aiBudgetWarnedDay {
			h.aiBudgetWarnedDay = false
		}
	}
}

// shouldLogError returns true if (path, errMsg) hasn't been logged in this
// process lifetime, and records it. Used to suppress repeat ERROR spam from
// retry loops hammering a deterministically-unparseable file.
func (h *MediaHandler) shouldLogError(path, errMsg string) bool {
	if h.loggedErrors == nil {
		return true
	}
	key := path + "\x00" + errMsg
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, seen := h.loggedErrors[key]; seen {
		return false
	}
	h.loggedErrors[key] = struct{}{}
	return true
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

// isInsideLibrary returns true if the file path resides within any configured
// library directory. Used to skip re-processing files that are already
// organized when watch paths accidentally overlap with library paths.
func (h *MediaHandler) isInsideLibrary(path string) bool {
	for _, lib := range h.tvLibraries {
		if strings.HasPrefix(path, filepath.Clean(lib)+string(filepath.Separator)) {
			return true
		}
	}
	for _, lib := range h.movieLibs {
		if strings.HasPrefix(path, filepath.Clean(lib)+string(filepath.Separator)) {
			return true
		}
	}
	return false
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

	// Skip paths within the deterministic-unparseable backoff window. The
	// periodic scanner re-walks watch folders every few minutes; without this
	// guard, a single unparseable release (obfuscated SAB hash, season-pack
	// release without episode markers, numeric-token false positive) would
	// be re-attempted forever.
	if deferred, remaining, lastErr := h.unparseableCache.IsDeferred(path); deferred {
		h.logger.Debug("handler", "Skipping deferred unparseable file",
			logging.F("path", path),
			logging.F("remaining", remaining.String()),
			logging.F("last_error", lastErr))
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
	var parsedSeason, parsedEpisode int
	parseMethod := activity.MethodRegex
	aiConfidence := 0.0

	isObfuscated := naming.IsObfuscatedFilename(filename)
	if isObfuscated {
		h.logger.Info("handler", "Detected obfuscated filename, using folder name", logging.F("filename", filename))
	}

	sourceHint := h.getSourceHint(path)
	isTVEpisode := naming.IsTVEpisodeFromPath(path, sourceHint)

	// Insert one parse decision row per debounced processing attempt.  We
	// look up any prior decision for this source path so the audit can
	// surface how the same file was previously parsed (ExistingMatchMethod).
	var decisionID int64
	if h.db != nil {
		mediaTypeGuessed := "movie"
		if isTVEpisode {
			mediaTypeGuessed = "tv"
		}
		var existingMethod string
		if prior, perr := h.db.GetMostRecentDecisionBySourcePath(path); perr == nil && prior != nil {
			existingMethod = prior.ParseMethod
		}
		var insertErr error
		decisionID, insertErr = h.db.InsertDecision(database.ParseDecision{
			SourcePath:          path,
			SourceFilename:      filepath.Base(path),
			EventAt:             time.Now().UTC(),
			MediaTypeGuessed:    mediaTypeGuessed,
			ExistingMatchMethod: existingMethod,
		})
		if insertErr != nil {
			h.logger.Warn("handler", "failed to insert parse decision",
				logging.F("filename", filename),
				logging.F("error", insertErr.Error()))
		}
	}

	if isTVEpisode {
		if len(h.tvLibraries) == 0 {
			h.logger.Warn("handler", "No TV libraries configured, skipping", logging.F("filename", filename))
			h.updateDecisionOrganize(decisionID, nil, fmt.Errorf("no TV libraries configured"))
			return
		}
		mediaType = notify.MediaTypeTVEpisode

		tvInfo, strippedTokens, parseErr := naming.ParseTVShowFromPathVerbose(path)
		if parseErr == nil {
			parsedTitle = tvInfo.Title
			parsedSeason = tvInfo.Season
			parsedEpisode = tvInfo.Episode
			if tvInfo.Year != "" {
				year := 0
				if _, err := fmt.Sscanf(tvInfo.Year, "%d", &year); err == nil {
					parsedYear = &year
				}
			}

			if h.db != nil && decisionID != 0 {
				u := database.ParseUpdate{
					ParseMethod:      string(activity.MethodRegex),
					ParsedTitle:      tvInfo.Title,
					ParsedYear:       parsedYear,
					MediaTypeGuessed: "tv",
				}
				if tvInfo.Season != 0 {
					s := tvInfo.Season
					u.ParsedSeason = &s
				}
				if tvInfo.Episode != 0 {
					e := tvInfo.Episode
					u.ParsedEpisode = &e
				}
				if b, jerr := json.Marshal(strippedTokens); jerr == nil {
					u.ParserStrippedTokens = string(b)
				}
				if updateErr := h.db.UpdateParse(decisionID, u); updateErr != nil {
					h.logger.Warn("handler", "failed to update parse decision",
						logging.F("filename", filename),
						logging.F("error", updateErr.Error()))
				}
			}

			confidence := naming.CalculateTitleConfidence(tvInfo.Title, filename)
			if h.shouldQueueForAI(path, filename, tvInfo, nil, confidence) {
				h.markDecisionQueued(decisionID)
				h.queueForAI(path, filename, tvInfo, nil, "tv", confidence, "", decisionID)
				return
			}
			if h.aiEnabled && confidence < h.aiConfig.AutoTriggerThreshold {
				h.logger.Info("handler", "AI enhancement skipped for deterministic TV parse",
					logging.F("filename", filename),
					logging.F("confidence", confidence))
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
			h.updateDecisionOrganize(decisionID, nil, fmt.Errorf("no movie libraries configured"))
			return
		}
		targetLib = h.movieLibs[0]
		mediaType = notify.MediaTypeMovie

		movieInfo, strippedTokens, parseErr := naming.ParseMovieFromPathVerbose(path)
		if parseErr == nil {
			parsedTitle = movieInfo.Title
			if movieInfo.Year != "" {
				year := 0
				if _, err := fmt.Sscanf(movieInfo.Year, "%d", &year); err == nil {
					parsedYear = &year
				}
			}

			if h.db != nil && decisionID != 0 {
				u := database.ParseUpdate{
					ParseMethod:      string(activity.MethodRegex),
					ParsedTitle:      movieInfo.Title,
					ParsedYear:       parsedYear,
					MediaTypeGuessed: "movie",
				}
				if b, jerr := json.Marshal(strippedTokens); jerr == nil {
					u.ParserStrippedTokens = string(b)
				}
				if updateErr := h.db.UpdateParse(decisionID, u); updateErr != nil {
					h.logger.Warn("handler", "failed to update parse decision",
						logging.F("filename", filename),
						logging.F("error", updateErr.Error()))
				}
			}

			confidence := naming.CalculateTitleConfidence(movieInfo.Title, filename)
			if h.shouldQueueForAI(path, filename, nil, movieInfo, confidence) {
				h.markDecisionQueued(decisionID)
				h.queueForAI(path, filename, nil, movieInfo, "movie", confidence, targetLib, decisionID)
				return
			}
			if h.aiEnabled && confidence < h.aiConfig.AutoTriggerThreshold {
				h.logger.Info("handler", "AI enhancement skipped for deterministic movie parse",
					logging.F("filename", filename),
					logging.F("confidence", confidence))
			}
		}

		if !h.checkTargetHealth(targetLib) {
			h.logger.Warn("handler", "Target library unhealthy, skipping", logging.F("filename", filename), logging.F("target", targetLib))
			h.updateDecisionOrganize(decisionID, nil, fmt.Errorf("target library unhealthy: %s", targetLib))
			return
		}

		result, err = h.movieOrganizer.OrganizeMovie(path, targetLib)
	}

	duration := time.Since(startTime)

	h.updateDecisionOrganize(decisionID, result, err)

	// Track notification results
	sonarrNotified := false
	radarrNotified := false

	if err != nil {
		if IsDeterministicUnparseable(err.Error()) {
			h.unparseableCache.Record(path, err.Error())
		}
		if h.shouldLogError(path, err.Error()) {
			h.logger.Error("handler", "Organization failed", err, logging.F("filename", filename))
		}
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
		yearStr := ""
		if parsedYear != nil {
			yearStr = fmt.Sprintf("%d", *parsedYear)
		}
		sonarrNotified, radarrNotified = h.sendNotificationsWithTracking(result, mediaType, parsedTitle, yearStr, parsedSeason, parsedEpisode)
		h.cleanupSourceDir(path)
		h.unparseableCache.Forget(path)
	} else if result.Skipped {
		h.logger.Info("handler", "Skipped", logging.F("filename", filename), logging.F("reason", result.SkipReason))
	} else {
		errMsg := ""
		if result.Error != nil {
			errMsg = result.Error.Error()
		}
		if IsDeterministicUnparseable(errMsg) {
			h.unparseableCache.Record(path, errMsg)
		}
		if h.shouldLogError(path, errMsg) {
			h.logger.Error("handler", "Organization failed", result.Error, logging.F("filename", filename))
		}
		h.stats.RecordError()
	}

	// Log activity entry after notifications
	h.logEntry(result, mediaType, parsedTitle, parsedYear, parseMethod, aiConfidence, duration, sonarrNotified, radarrNotified)
}

func (h *MediaHandler) shouldQueueForAI(path, filename string, tvInfo *naming.TVShowInfo, movieInfo *naming.MovieInfo, confidence float64) bool {
	if !h.aiEnabled || confidence >= h.aiConfig.AutoTriggerThreshold {
		return false
	}
	if hasDeterministicTVEpisodeIdentity(path, tvInfo) {
		return false
	}
	if hasDeterministicMovieIdentity(filename, movieInfo) {
		return false
	}
	return true
}

func hasDeterministicTVEpisodeIdentity(path string, tvInfo *naming.TVShowInfo) bool {
	if tvInfo == nil || strings.TrimSpace(tvInfo.Title) == "" || tvInfo.Season <= 0 || tvInfo.Episode <= 0 {
		return false
	}
	return naming.IsTVEpisodeFromPath(path, naming.SourceUnknown)
}

func hasDeterministicMovieIdentity(filename string, movieInfo *naming.MovieInfo) bool {
	if movieInfo == nil || strings.TrimSpace(movieInfo.Title) == "" || movieInfo.Year == "" {
		return false
	}
	return !naming.IsObfuscatedFilename(filename)
}

func (h *MediaHandler) queueForAI(path, filename string, tvInfo *naming.TVShowInfo, movieInfo *naming.MovieInfo, mediaType string, confidence float64, targetLib string, decisionID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Don't re-queue blacklisted items
	if existing, ok := h.pendingAI[path]; ok && existing.Blacklisted {
		h.logger.Info("handler", "Skipping blacklisted AI item",
			logging.F("filename", filename))
		return
	}

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
		Path:            path,
		Filename:        filename,
		TVInfo:          tvInfo,
		MovieInfo:       movieInfo,
		MediaType:       mediaType,
		Confidence:      confidence,
		QueuedAt:        time.Now(),
		TargetLib:       targetLib,
		ParseDecisionID: decisionID,
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

func (h *MediaHandler) sendNotificationsWithTracking(result *organizer.OrganizationResult, mediaType notify.MediaType, title, year string, season, episode int) (sonarrNotified, radarrNotified bool) {
	if h.notifyManager == nil {
		return false, false
	}

	event := notify.OrganizationEvent{
		MediaType:   mediaType,
		SourcePath:  result.SourcePath,
		TargetPath:  result.TargetPath,
		TargetDir:   filepath.Dir(result.TargetPath),
		Title:       title,
		Year:        year,
		Season:      season,
		Episode:     episode,
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
	err := transfer.CheckDiskHealthForTransfer("", targetLib, 30*time.Second, 0)
	healthy := err == nil
	h.recordTargetHealth(targetLib, healthy, err)
	if !healthy {
		h.logger.Warn("handler", "Health check failed", logging.F("target", targetLib), logging.F("error", err.Error()))
	}
	return healthy
}

// recordTargetHealth logs a WARN (unhealthy→healthy) or INFO (healthy after
// failure) when a target library's health flips. Suppresses per-check noise
// while surfacing actionable state transitions.
func (h *MediaHandler) recordTargetHealth(target string, healthy bool, err error) {
	if target == "" {
		return
	}
	h.mu.Lock()
	prev, seen := h.targetHealthState[target]
	h.targetHealthState[target] = healthy
	h.mu.Unlock()
	if seen && prev == healthy {
		return
	}
	if !healthy {
		reason := ""
		if err != nil {
			reason = err.Error()
		}
		h.logger.Warn("handler", "Target library became unhealthy",
			logging.F("target", target),
			logging.F("reason", reason))
	} else if seen {
		h.logger.Info("handler", "Target library recovered",
			logging.F("target", target))
	}
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

// removeNonVideoContents walks dir and removes every non-media file and
// then every empty descendant directory. Returns (removed, kept) — "kept"
// is the count of media files encountered, i.e. files we refused to
// delete. A caller seeing kept > 0 must abort the cleanup: new content
// arrived between the initial containsVideoFilesRecursive check and now.
// This closes the TOCTOU window that a naive os.RemoveAll would expose.
func (h *MediaHandler) removeNonVideoContents(dir string) (removed, kept int) {
	// Delete leaves first (post-order) so that empty directories can be
	// removed after their contents are gone.
	type entry struct {
		path  string
		isDir bool
	}
	var all []entry
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == dir {
			return nil
		}
		all = append(all, entry{path: path, isDir: d.IsDir()})
		return nil
	})
	// Process deepest paths first by length (approximate post-order).
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if e.isDir {
			if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
				// Non-empty directory — an un-removable media file lives
				// inside; let the caller see kept>0 and abort.
			}
			continue
		}
		if h.IsMediaFile(e.path) {
			kept++
			continue
		}
		if err := os.Remove(e.path); err == nil {
			removed++
		}
	}
	return removed, kept
}

// cleanupSourceDir removes the source download directory after a successful move,
// walking up parent directories until it reaches a watch root.
//
// Gate: if sourcePath still exists the source was preserved (keepSource, dry-run),
// so cleanup is skipped entirely.
//
// For the starting release directory only, organizer.PurgeNonAllowed is called to
// strip junk files while preserving allowed video and subtitle extensions. Parent
// directories are only removed if they are already empty; junk in parents is never
// touched to avoid interfering with other concurrent downloads.
func (h *MediaHandler) cleanupSourceDir(sourcePath string) {
	// Gate: source still present means keepSource or dry-run — do nothing.
	if _, err := os.Stat(sourcePath); err == nil {
		return
	}

	// Allowlist gate: only purge a source directory when there is a recent
	// successful parse_decisions row for this exact source path.  Without
	// this, any file that disappears for an unrelated reason (manual rm,
	// transient mount loss, racing daemon) would cause us to delete the
	// surrounding release directory.
	if h.db != nil {
		ok, err := h.db.HasRecentSuccessForSource(sourcePath, 24*time.Hour)
		if err != nil {
			h.logger.Warn("handler", "cleanup gate query failed; aborting cleanup",
				logging.F("path", sourcePath), logging.F("error", err.Error()))
			return
		}
		if !ok {
			h.logger.Info("handler", "Skipping cleanup: no recent SUCCESS parse_decisions row",
				logging.F("path", sourcePath))
			return
		}
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
	firstDir := true
	for {
		clean := filepath.Clean(dir)
		if _, isRoot := rootSet[clean]; isRoot {
			return // never remove a watch root
		}

		if firstDir {
			// Purge only disallowed files from the starting release directory.
			// Subtitle and video files are preserved by the allowlist.
			if err := organizer.PurgeNonAllowed(dir); err != nil {
				h.logger.Warn("handler", "Allowlist purge failed",
					logging.F("dir", dir), logging.F("error", err.Error()))
			}
			firstDir = false
		}

		if h.containsVideoFilesRecursive(dir) {
			return // media file present — leave directory alone
		}

		// os.Remove only succeeds on an empty directory; if any allowed file
		// (subtitle, video sibling) or unexpected content remains, we stop.
		if err := os.Remove(dir); err != nil {
			if !os.IsNotExist(err) {
				h.logger.Info("handler", "Directory not empty, stopping cleanup",
					logging.F("dir", dir))
			}
			return
		}
		h.logger.Info("handler", "Cleaned up source directory", logging.F("dir", dir))

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
	rawPath := strings.TrimSpace(event.ItemPath)
	path := h.pathTranslator.JellyfinToDaemon(rawPath)
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
			if dec, err := h.db.GetUnresolvedDecisionByTargetPath(path); err != nil {
				if h.logger != nil {
					h.logger.Warn("handler", "Failed to query parse decision for path", logging.F("path", path), logging.F("error", err.Error()))
				}
			} else if dec != nil {
				now := time.Now().UTC()
				if updateErr := h.db.UpdateOutcome(dec.ID, database.OutcomeUpdate{
					JellyfinItemID:     itemID,
					JellyfinImdbID:     event.ProviderImdb,
					JellyfinTmdbID:     event.ProviderTmdb,
					JellyfinTvdbID:     event.ProviderTvdb,
					JellyfinResolvedAt: &now,
				}); updateErr != nil {
					if h.logger != nil {
						h.logger.Warn("handler", "Failed to update parse decision outcome", logging.F("path", path), logging.F("decision_id", dec.ID), logging.F("error", updateErr.Error()))
					}
				}
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

func (h *MediaHandler) ProcessPendingAI(ctx context.Context) {
	h.mu.Lock()
	items := make([]*PendingItem, 0, len(h.pendingAI))
	var blacklistedCount int
	var oldestQueuedAt time.Time
	for _, item := range h.pendingAI {
		items = append(items, item)
		if item.Blacklisted {
			blacklistedCount++
		}
		if oldestQueuedAt.IsZero() || item.QueuedAt.Before(oldestQueuedAt) {
			oldestQueuedAt = item.QueuedAt
		}
	}
	h.mu.Unlock()

	// Snapshot the queue so operators can catch memory leaks (the audit
	// flagged blacklisted items never being evicted — this surfaces that).
	if len(items) > 0 {
		oldestAge := time.Duration(0)
		if !oldestQueuedAt.IsZero() {
			oldestAge = time.Since(oldestQueuedAt).Round(time.Second)
		}
		h.logger.Info("handler", "AI queue snapshot",
			logging.F("pending", len(items)),
			logging.F("blacklisted", blacklistedCount),
			logging.F("oldest_age", oldestAge.String()))
	}

	now := time.Now()
	expiry := 24 * time.Hour

	for _, item := range items {
		// Expire old items — fall back to regex result rather than abandoning the file
		if now.Sub(item.QueuedAt) > expiry {
			h.logger.Info("handler", "AI enhancement timed out after 24h, falling back to regex",
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
			h.organizeWithRegexFallback(item)
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

		// Skip blacklisted items, but expire them after 24h so that if
		// upstream (Ollama subscription, network, etc.) recovers we get
		// another attempt rather than leaking the item in memory forever.
		if item.Blacklisted {
			if now.Sub(item.LastAttemptAt) < 24*time.Hour {
				continue
			}
			item.Blacklisted = false
			item.AttemptCount = 0
			h.logger.Info("handler", "Un-blacklisting AI item for retry after cooldown",
				logging.F("filename", item.Filename))
		}

		// Exponential backoff: skip items that haven't waited long enough since last attempt
		if item.AttemptCount > 0 {
			backoff := aiRetryBackoff(item.AttemptCount)
			if now.Sub(item.LastAttemptAt) < backoff {
				continue
			}
		}

		// Check rate limit
		if h.aiRateLimiter != nil && !h.aiRateLimiter.Allow() {
			if h.enhanceLogger != nil {
				h.mu.Lock()
				pendingCount := len(h.pendingAI)
				h.mu.Unlock()
				hourly, daily := h.aiRateLimiter.Stats()
				h.enhanceLogger.Log(EnhanceLogEntry{
					Action:       "rate_limited",
					PendingCount: pendingCount,
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

		// Cache check before upstream call — avoids re-parsing identical
		// filenames (common with bulk imports / re-queued items) and saves
		// rate-limiter budget. Cache keys use NormalizeInput so minor case
		// or spacing differences don't cause misses.
		var aiResult *ai.Result
		var err error
		var fromCache bool
		if h.aiCache != nil {
			normalized := ai.NormalizeInput(item.Filename)
			if cached, cerr := h.aiCache.Get(normalized, item.MediaType, h.aiConfig.Model); cerr == nil && cached != nil {
				aiResult = cached
				fromCache = true
				h.logger.Debug("handler", "AI cache hit",
					logging.F("filename", item.Filename))
			}
		}
		if !fromCache {
			aiResult, err = h.aiMatcher.ParseWithRetry(ctx, item.Filename)
		}

		// Check for permanent error first — those don't consume AI budget.
		var httpErr *ai.HTTPError
		permanent := err != nil && errors.As(err, &httpErr) && httpErr.IsPermanent()

		// Only count successful calls against the budget. The earlier logic
		// recorded every non-permanent attempt, so a burst of transient
		// failures could exhaust the daily cap without a single useful
		// result. Permanent errors are free (by design); other failures are
		// also free now — they'll retry under backoff.
		if h.aiRateLimiter != nil && err == nil {
			hUsed, hCap, dUsed, dCap := h.aiRateLimiter.RecordAndReport()
			h.logger.Debug("handler", "AI budget consumed",
				logging.F("filename", item.Filename),
				logging.F("hourly", fmt.Sprintf("%d/%d", hUsed, hCap)),
				logging.F("daily", fmt.Sprintf("%d/%d", dUsed, dCap)))
			h.emitAIBudgetWarnings(hUsed, hCap, dUsed, dCap)
		} else if permanent {
			h.logger.Debug("handler", "AI budget skipped (permanent error)",
				logging.F("filename", item.Filename),
				logging.F("status", httpErr.StatusCode))
		}

		if err != nil {
			item.AttemptCount++
			item.LastAttemptAt = time.Now()

			if permanent || item.AttemptCount >= maxAIRetryAttempts {
				item.Blacklisted = true
				reason := fmt.Sprintf("failed %d attempts", item.AttemptCount)
				permanentKey := ""
				if permanent {
					reason = fmt.Sprintf("permanent error (HTTP %d): %s", httpErr.StatusCode, httpErr.Body)
					permanentKey = fmt.Sprintf("%d:%s", httpErr.StatusCode, httpErr.Body)
				}
				// First time we've seen this permanent error → WARN. Repeats
				// from the same subscription/quota/auth state → INFO so the
				// fallback stays visible without flooding the log.
				h.mu.Lock()
				firstOccurrence := permanentKey == "" || permanentKey != h.lastAIPermanentKey
				if permanentKey != "" {
					h.lastAIPermanentKey = permanentKey
				}
				h.mu.Unlock()
				if firstOccurrence {
					h.logger.Warn("handler", "AI enhancement unavailable, falling back to regex",
						logging.F("filename", item.Filename),
						logging.F("reason", reason))
				} else {
					h.logger.Info("handler", "AI enhancement unavailable (repeat), falling back to regex",
						logging.F("filename", item.Filename),
						logging.F("reason", reason))
				}
				if h.enhanceLogger != nil {
					h.enhanceLogger.Log(EnhanceLogEntry{
						Action: "permanently_blacklisted",
						File:   item.Filename,
						Reason: reason,
					})
				}
				h.mu.Lock()
				delete(h.pendingAI, item.Path)
				h.mu.Unlock()
				h.organizeWithRegexFallback(item)
			} else {
				h.logger.Warn("handler", "AI enhancement failed, will retry with backoff",
					logging.F("filename", item.Filename),
					logging.F("attempt", item.AttemptCount),
					logging.F("error", err.Error()))
				if h.enhanceLogger != nil {
					h.enhanceLogger.Log(EnhanceLogEntry{
						Action: "ai_retry_scheduled",
						File:   item.Filename,
						Reason: fmt.Sprintf("attempt %d failed: %s", item.AttemptCount, err.Error()),
					})
				}
			}
			continue
		}

		// Cache successful upstream results so re-queued identical
		// filenames skip the network round-trip next time.
		if !fromCache && h.aiCache != nil {
			normalized := ai.NormalizeInput(item.Filename)
			_ = h.aiCache.Put(normalized, item.MediaType, h.aiConfig.Model, aiResult, 0)
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
				entry := EnhanceLogEntry{
					Action:       "flagged_for_review",
					File:         item.Filename,
					RegexTitle:   regexTitle,
					AITitle:      aiResult.Title,
					AIConfidence: aiResult.Confidence,
					Category:     string(classification.Category),
					Reason:       reason,
					MediaType:    item.MediaType,
					SourcePath:   item.Path,
					TargetLib:    item.TargetLib,
				}
				if aiResult.Year != nil {
					entry.AIYear = aiResult.Year.Int()
				}
				if aiResult.Season != nil {
					entry.AISeason = aiResult.Season.Int()
				}
				if len(aiResult.Episodes) > 0 {
					ep := aiResult.Episodes[0]
					entry.AIEpisode = &ep
				}
				h.enhanceLogger.Log(entry)
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

// markDecisionQueued sets organize_outcome='queued' on a decision row that
// has been handed off to the AI enhancement queue.  Without this marker the
// row would remain organize_outcome=NULL forever if the daemon restarts
// before the AI worker completes.
func (h *MediaHandler) markDecisionQueued(id int64) {
	if h.db == nil || id == 0 {
		return
	}
	if err := h.db.MarkDecisionQueued(id); err != nil {
		h.logger.Warn("handler", "failed to mark parse decision queued",
			logging.F("decision_id", id),
			logging.F("error", err.Error()))
	}
}

// updateDecisionOrganize updates the organize columns of a parse decision row.
// It is a no-op when the DB is not configured or the decision ID is zero.
func (h *MediaHandler) updateDecisionOrganize(id int64, result *organizer.OrganizationResult, err error) {
	if h.db == nil || id == 0 {
		return
	}

	u := database.OrganizeUpdate{}
	now := time.Now().UTC()

	switch {
	case err != nil:
		u.OrganizeOutcome = "failed"
		u.OrganizeError = err.Error()
	case result == nil:
		u.OrganizeOutcome = "failed"
		u.OrganizeError = "nil result"
	case result.Success:
		u.OrganizeOutcome = "success"
		u.TargetPath = result.TargetPath
		u.TargetAt = &now
	case result.Skipped:
		u.OrganizeOutcome = "skipped"
		u.TargetPath = result.TargetPath
		// l1: leave TargetAt nil for skipped paths.  No write happened, so
		// stamping target_at would mislead the auditor into thinking a copy
		// landed at this path at this moment.
	default:
		u.OrganizeOutcome = "failed"
		if result.Error != nil {
			u.OrganizeError = result.Error.Error()
		}
	}

	if updateErr := h.db.UpdateOrganize(id, u); updateErr != nil {
		h.logger.Warn("handler", "failed to update parse decision organize columns",
			logging.F("decision_id", id),
			logging.F("error", updateErr.Error()))
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

	if h.db != nil && item.ParseDecisionID != 0 {
		u := database.ParseUpdate{
			ParseMethod:      string(activity.MethodAI),
			ParsedTitle:      aiResult.Title,
			MediaTypeGuessed: item.MediaType,
		}
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			y := *aiResult.Year.Int()
			u.ParsedYear = &y
		}
		if aiResult.Season != nil && aiResult.Season.Int() != nil {
			s := *aiResult.Season.Int()
			u.ParsedSeason = &s
		}
		if len(aiResult.Episodes) > 0 {
			e := aiResult.Episodes[0]
			u.ParsedEpisode = &e
		}
		if updateErr := h.db.UpdateParse(item.ParseDecisionID, u); updateErr != nil {
			h.logger.Warn("handler", "failed to update parse decision",
				logging.F("filename", item.Filename),
				logging.F("error", updateErr.Error()))
		}
	}

	h.updateDecisionOrganize(item.ParseDecisionID, result, err)

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

		// Send notifications for AI-enhanced moves (was previously missing)
		var mediaType notify.MediaType
		var season, episode int
		yearStr := ""
		if item.MediaType == "tv" {
			mediaType = notify.MediaTypeTVEpisode
			if aiResult.Season != nil && aiResult.Season.Int() != nil {
				season = *aiResult.Season.Int()
			}
			if len(aiResult.Episodes) > 0 {
				episode = aiResult.Episodes[0]
			}
		} else {
			mediaType = notify.MediaTypeMovie
		}
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			yearStr = fmt.Sprintf("%d", *aiResult.Year.Int())
		}
		sonarrNotified, radarrNotified := h.sendNotificationsWithTracking(result, mediaType, aiResult.Title, yearStr, season, episode)

		// Activity-log the AI-enhanced move so operators can see what actually
		// ran via the AI path (previously only the regex path wrote entries).
		var parsedYear *int
		if aiResult.Year != nil && aiResult.Year.Int() != nil {
			y := *aiResult.Year.Int()
			parsedYear = &y
		}
		h.logEntry(result, mediaType, aiResult.Title, parsedYear, activity.MethodAI, aiResult.Confidence, time.Since(item.QueuedAt), sonarrNotified, radarrNotified)

		// Record AI improvement in tracking table
		if h.db != nil {
			now := time.Now()
			imp := &database.AIImprovement{
				RequestID:    fmt.Sprintf("ai-%s-%d", filepath.Base(item.Path), now.UnixNano()),
				Filename:     item.Filename,
				AITitle:      &aiResult.Title,
				AIType:       &item.MediaType,
				AIConfidence: &aiResult.Confidence,
				Status:       "completed",
				Attempts:     item.AttemptCount,
				CompletedAt:  &now,
			}
			if aiResult.Year != nil && aiResult.Year.Int() != nil {
				imp.AIYear = aiResult.Year.Int()
			}
			if err := h.db.UpsertAIImprovement(imp); err != nil {
				h.logger.Error("handler", "failed to record AI improvement", err,
					logging.F("filename", item.Filename))
			}
		}

		h.cleanupSourceDir(item.Path)
	}
}

// organizeWithRegexFallback runs the standard regex+Sonarr organize path for
// an item whose AI enhancement failed or was abandoned. The item's regex parse
// was already below the AI auto-trigger threshold, so this is the best-effort
// path when AI is unavailable — better than leaving files stuck indefinitely.
func (h *MediaHandler) organizeWithRegexFallback(item *PendingItem) {
	if item == nil || item.Path == "" {
		return
	}
	if _, err := os.Stat(item.Path); os.IsNotExist(err) {
		return
	}

	startTime := time.Now()
	filename := item.Filename

	var result *organizer.OrganizationResult
	var err error
	var mediaType notify.MediaType
	var parsedTitle string
	var parsedYear *int
	var parsedSeason, parsedEpisode int

	if item.MediaType == "tv" {
		mediaType = notify.MediaTypeTVEpisode
		if item.TVInfo != nil {
			parsedTitle = item.TVInfo.Title
			parsedSeason = item.TVInfo.Season
			parsedEpisode = item.TVInfo.Episode
			if item.TVInfo.Year != "" {
				year := 0
				if _, e := fmt.Sscanf(item.TVInfo.Year, "%d", &year); e == nil {
					parsedYear = &year
				}
			}
		}
		result, err = h.tvOrganizer.OrganizeTVEpisodeAuto(item.Path, func(p string) (int64, error) {
			info, statErr := os.Stat(p)
			if statErr != nil {
				return 0, statErr
			}
			return info.Size(), nil
		})
	} else {
		mediaType = notify.MediaTypeMovie
		if item.MovieInfo != nil {
			parsedTitle = item.MovieInfo.Title
			if item.MovieInfo.Year != "" {
				year := 0
				if _, e := fmt.Sscanf(item.MovieInfo.Year, "%d", &year); e == nil {
					parsedYear = &year
				}
			}
		}
		targetLib := item.TargetLib
		if targetLib == "" && len(h.movieLibs) > 0 {
			targetLib = h.movieLibs[0]
		}
		if targetLib == "" {
			h.logger.Warn("handler", "No movie libraries configured for fallback", logging.F("filename", filename))
			h.stats.RecordError()
			h.logEntry(&organizer.OrganizationResult{
				Success:    false,
				SourcePath: item.Path,
				Error:      fmt.Errorf("no movie libraries configured"),
			}, mediaType, parsedTitle, parsedYear, activity.MethodRegex, 0.0, time.Since(startTime), false, false)
			return
		}
		if !h.checkTargetHealth(targetLib) {
			h.logger.Warn("handler", "Target library unhealthy, deferring fallback",
				logging.F("filename", filename), logging.F("target", targetLib))
			h.stats.RecordError()
			h.logEntry(&organizer.OrganizationResult{
				Success:    false,
				SourcePath: item.Path,
				Skipped:    true,
				SkipReason: "target library unhealthy",
				Error:      fmt.Errorf("target library unhealthy: %s", targetLib),
			}, mediaType, parsedTitle, parsedYear, activity.MethodRegex, 0.0, time.Since(startTime), false, false)
			return
		}
		result, err = h.movieOrganizer.OrganizeMovie(item.Path, targetLib)
	}

	duration := time.Since(startTime)
	sonarrNotified := false
	radarrNotified := false

	h.updateDecisionOrganize(item.ParseDecisionID, result, err)

	if err != nil {
		h.logger.Error("handler", "Regex-fallback organization failed", err, logging.F("filename", filename))
		h.stats.RecordError()
		h.logEntry(result, mediaType, parsedTitle, parsedYear, activity.MethodRegex, 0.0, duration, sonarrNotified, radarrNotified)
		return
	}

	if result != nil && result.Success {
		h.logger.Info("handler", "Regex-fallback organized successfully",
			logging.F("source", filepath.Base(result.SourcePath)),
			logging.F("target", result.TargetPath),
			logging.F("size_mb", float64(result.BytesCopied)/(1024*1024)),
			logging.F("duration", result.Duration.String()))
		if mediaType == notify.MediaTypeMovie {
			h.stats.RecordMovie(result.BytesCopied)
		} else {
			h.stats.RecordTV(result.BytesCopied)
		}
		yearStr := ""
		if parsedYear != nil {
			yearStr = fmt.Sprintf("%d", *parsedYear)
		}
		sonarrNotified, radarrNotified = h.sendNotificationsWithTracking(result, mediaType, parsedTitle, yearStr, parsedSeason, parsedEpisode)
		h.cleanupSourceDir(item.Path)
	} else if result != nil && result.Skipped {
		h.logger.Info("handler", "Regex-fallback skipped", logging.F("filename", filename), logging.F("reason", result.SkipReason))
	} else if result != nil && result.Error != nil {
		h.logger.Error("handler", "Regex-fallback organization failed", result.Error, logging.F("filename", filename))
		h.stats.RecordError()
	}

	h.logEntry(result, mediaType, parsedTitle, parsedYear, activity.MethodRegex, 0.0, duration, sonarrNotified, radarrNotified)
}

func (h *MediaHandler) PruneActivityLogs(days int) error {
	if h.activityLogger == nil {
		return nil
	}
	return h.activityLogger.PruneOld(days)
}
