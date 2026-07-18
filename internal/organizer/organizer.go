package organizer

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/analyzer"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/identity"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/library"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/quality"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
	syncsvc "github.com/Nomadcxx/plex2jellyfin/internal/sync"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/Nomadcxx/plex2jellyfin/internal/video"
)

type OrganizationResult struct {
	Success         bool
	SourcePath      string
	TargetPath      string
	BytesCopied     int64
	Duration        time.Duration
	Attempts        int
	Error           error
	Skipped         bool
	SkipReason      string
	SourceQuality   *quality.QualityInfo
	ExistingQuality *quality.QualityInfo
}

type SeasonPackResult struct {
	Success     bool
	SourceDir   string
	Imported    []*OrganizationResult
	Unresolved  []string
	Skipped     bool
	SkipReason  string
	Error       error
	BytesCopied int64
}

type Organizer struct {
	dryRun         bool
	keepSource     bool
	forceOverwrite bool
	libraries      []string
	selector       *library.Selector
	transferer     transfer.Transferer
	timeout        time.Duration
	checksumVerify bool
	targetUID      int
	targetGID      int
	fileMode       os.FileMode
	dirMode        os.FileMode
	sonarrClient   *sonarr.Client
	jellyfinClient *jellyfin.Client
	playbackSafety bool
	db             *database.MediaDB
	syncService    *syncsvc.SyncService
	pluginClient   *jellyfin.PluginClient
	pauseOnScan    bool
	paused         bool
	pauseMu        sync.RWMutex
	playbackLocks  *jellyfin.PlaybackLockManager
	deferredQueue  *jellyfin.DeferredQueue
}

func NewOrganizer(libraries []string, options ...func(*Organizer)) (*Organizer, error) {
	transferer, err := transfer.New(transfer.BackendAuto)
	if err != nil {
		return nil, fmt.Errorf("failed to create transferer: %w", err)
	}

	org := &Organizer{
		dryRun:         false,
		keepSource:     false,
		forceOverwrite: false,
		libraries:      libraries,
		transferer:     transferer,
		timeout:        5 * time.Minute,
		checksumVerify: false,
		targetUID:      -1,
		targetGID:      -1,
		fileMode:       0,
		dirMode:        0,
	}

	// Apply options first (to get sonarrClient and db if provided)
	for _, opt := range options {
		opt(org)
	}

	// Create selector with Sonarr and database integration if available
	org.selector = library.NewSelectorWithConfig(library.SelectorConfig{
		Libraries:     libraries,
		SonarrClient:  org.sonarrClient,
		CacheDuration: 5 * time.Minute,
		DB:            org.db,
	})

	return org, nil
}

// WithDryRun sets dry run mode
func WithDryRun(dryRun bool) func(*Organizer) {
	return func(o *Organizer) {
		o.dryRun = dryRun
	}
}

// WithKeepSource sets whether to keep source files
func WithKeepSource(keep bool) func(*Organizer) {
	return func(o *Organizer) {
		o.keepSource = keep
	}
}

// WithTimeout sets the transfer timeout
func WithTimeout(timeout time.Duration) func(*Organizer) {
	return func(o *Organizer) {
		o.timeout = timeout
	}
}

func WithChecksumVerify(verify bool) func(*Organizer) {
	return func(o *Organizer) {
		o.checksumVerify = verify
	}
}

func WithBackend(backend transfer.Backend) func(*Organizer) {
	return func(o *Organizer) {
		t, err := transfer.New(backend)
		if err != nil {
			return
		}
		o.transferer = t
	}
}

// WithTransferer injects a pre-built transferer (e.g. one wrapped with a
// shared VolumeLimiter) so multiple organizers can coordinate on a single
// per-destination concurrency budget.
func WithTransferer(t transfer.Transferer) func(*Organizer) {
	return func(o *Organizer) {
		if t != nil {
			o.transferer = t
		}
	}
}

func WithForceOverwrite(force bool) func(*Organizer) {
	return func(o *Organizer) {
		o.forceOverwrite = force
	}
}

func WithPermissions(uid, gid int, fileMode, dirMode os.FileMode) func(*Organizer) {
	return func(o *Organizer) {
		o.targetUID = uid
		o.targetGID = gid
		o.fileMode = fileMode
		o.dirMode = dirMode
	}
}

// WithSonarrClient sets the Sonarr client for intelligent library selection
func WithSonarrClient(client *sonarr.Client) func(*Organizer) {
	return func(o *Organizer) {
		o.sonarrClient = client
	}
}

// WithJellyfinClient enables playback-safety fallback checks through the Jellyfin sessions API.
func WithJellyfinClient(client *jellyfin.Client, playbackSafety bool) func(*Organizer) {
	return func(o *Organizer) {
		o.jellyfinClient = client
		o.playbackSafety = playbackSafety
	}
}

func WithPluginClient(client *jellyfin.PluginClient, pauseOnScan bool) func(*Organizer) {
	return func(o *Organizer) {
		o.pluginClient = client
		o.pauseOnScan = pauseOnScan
	}
}

// WithDatabase sets the HOLDEN database for self-learning
func WithDatabase(db *database.MediaDB) func(*Organizer) {
	return func(o *Organizer) {
		o.db = db
	}
}

func WithSyncService(svc *syncsvc.SyncService) func(*Organizer) {
	return func(o *Organizer) {
		o.syncService = svc
	}
}

// WithPlaybackLockManager enables playback safety checks against webhook lock state.
func WithPlaybackLockManager(mgr *jellyfin.PlaybackLockManager) func(*Organizer) {
	return func(o *Organizer) {
		o.playbackLocks = mgr
		o.playbackSafety = mgr != nil
	}
}

// WithDeferredQueue configures where playback-blocked operations should be enqueued.
func WithDeferredQueue(queue *jellyfin.DeferredQueue) func(*Organizer) {
	return func(o *Organizer) {
		o.deferredQueue = queue
	}
}

// alreadyOrganizedResult detects the duplicate-event race: a concurrent
// handler invocation already moved the source to the target, so this
// transfer's failure is not a real error. Returns nil when it was.
func alreadyOrganizedResult(sourcePath, targetPath string) *OrganizationResult {
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		return nil
	}
	st, err := os.Stat(targetPath)
	if err != nil || st.Size() == 0 {
		return nil
	}
	return &OrganizationResult{
		Success:    false,
		SourcePath: sourcePath,
		TargetPath: targetPath,
		Skipped:    true,
		SkipReason: "already_organized",
	}
}

func (o *Organizer) buildTransferOptions() transfer.TransferOptions {
	return transfer.TransferOptions{
		Timeout:       o.timeout,
		Checksum:      o.checksumVerify,
		RetryAttempts: 3,
		RetryDelay:    5 * time.Second,
		PreserveAttrs: true,
		TargetUID:     o.targetUID,
		TargetGID:     o.targetGID,
		FileMode:      o.fileMode,
		DirMode:       o.dirMode,
	}
}

func (o *Organizer) applyDirOwnership(path string) error {
	if o.dirMode != 0 {
		if err := os.Chmod(path, o.dirMode); err != nil {
			return err
		}
	}
	if o.targetUID >= 0 || o.targetGID >= 0 {
		uid, gid := o.targetUID, o.targetGID
		if uid < 0 {
			uid = -1
		}
		if gid < 0 {
			gid = -1
		}
		if err := os.Chown(path, uid, gid); err != nil {
			if os.Geteuid() != 0 {
				return nil
			}
			return err
		}
	}
	return nil
}

func (o *Organizer) checkPlaybackSafety(sourcePath string) error {
	return o.checkPlaybackSafetyWithOp(sourcePath, "", "")
}

func (o *Organizer) Pause() {
	o.pauseMu.Lock()
	defer o.pauseMu.Unlock()
	o.paused = true
}

func (o *Organizer) Resume() {
	o.pauseMu.Lock()
	defer o.pauseMu.Unlock()
	o.paused = false
}

func (o *Organizer) IsPaused() bool {
	o.pauseMu.RLock()
	defer o.pauseMu.RUnlock()
	return o.paused
}

func (o *Organizer) shouldWaitForScan() bool {
	if !o.pauseOnScan || o.pluginClient == nil {
		return false
	}

	scans, err := o.pluginClient.GetActiveScans()
	if err != nil {
		return false
	}

	return len(scans.Scans) > 0
}

func (o *Organizer) waitIfScanning(timeout time.Duration) error {
	if !o.pauseOnScan || o.pluginClient == nil {
		return nil
	}

	start := time.Now()
	for o.shouldWaitForScan() {
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for Jellyfin scan to complete")
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (o *Organizer) checkPlaybackSafetyWithOp(sourcePath, opType, targetPath string) error {
	if !o.playbackSafety {
		return nil
	}

	if o.playbackLocks != nil {
		lockedPath := sourcePath
		locked, info := o.playbackLocks.IsLocked(sourcePath)
		if !locked && strings.TrimSpace(targetPath) != "" {
			if targetLocked, targetInfo := o.playbackLocks.IsLocked(targetPath); targetLocked {
				lockedPath = targetPath
				locked = true
				info = targetInfo
			}
		}

		if locked && info != nil {
			if o.deferredQueue != nil && opType != "" {
				o.deferredQueue.Add(lockedPath, jellyfin.DeferredOp{
					Type:       opType,
					SourcePath: sourcePath,
					TargetPath: targetPath,
					Reason:     "blocked by active playback",
					DeferredAt: time.Now(),
				})
			}
			return fmt.Errorf("file is being streamed by %s on %s (via webhook), deferring operation", info.UserName, info.DeviceName)
		}
	}

	if o.jellyfinClient != nil {
		playing, session, err := o.jellyfinClient.IsPathBeingPlayed(sourcePath)
		if err != nil {
			return nil
		}
		if playing && session != nil {
			return fmt.Errorf("file is being streamed by %s on %s (via API), deferring operation", session.UserName, session.DeviceName)
		}
		if playing {
			return fmt.Errorf("file is being actively streamed (via API), deferring operation")
		}
	}

	return nil
}

func (o *Organizer) OrganizeMovie(sourcePath, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	movie, err := naming.ParseMovieFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse movie name: %w", err),
		}, nil
	}

	return o.OrganizeMovieWithParsed(sourcePath, libraryPath, *movie)
}

func (o *Organizer) OrganizeMovieWithParsed(sourcePath, libraryPath string, movie naming.MovieInfo) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	cleanName := naming.NormalizeMediaName(movie.Title, movie.Year)
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
			Success:         true,
			SourcePath:      sourcePath,
			TargetPath:      targetPath,
			SourceQuality:   sourceQuality,
			ExistingQuality: existingQuality,
		}, nil
	}

	opts := o.buildTransferOptions()

	var err error
	var result *transfer.TransferResult
	if o.keepSource {
		result, err = o.transferer.Copy(sourcePath, targetPath, opts)
	} else {
		result, err = o.transferer.Move(sourcePath, targetPath, opts)
	}

	if err != nil {
		if skipped := alreadyOrganizedResult(sourcePath, targetPath); skipped != nil {
			return skipped, nil
		}
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			BytesCopied:   result.BytesCopied,
			Duration:      result.Duration,
			Attempts:      result.Attempts,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("transfer failed: %w", err),
		}, nil
	}

	if existingFile != "" && existingFile != targetPath {
		if err := os.Remove(existingFile); err != nil && !os.IsNotExist(err) {
			log.Printf("[organizer] warning: failed to remove existing file %s: %v", existingFile, err)
		}
	}

	if o.db != nil {
		if mediaFile, err := o.db.GetMediaFile(sourcePath); err == nil && mediaFile != nil {
			mediaFile.Path = targetPath
			if err := o.db.UpdateMediaFile(mediaFile); err != nil {
				log.Printf("[organizer] warning: failed to update media file path after move: %v", err)
			}
		}
	}

	// HOLDEN Phase 3: Self-learning - update database with organized movie
	if o.db != nil {
		yearInt := 0
		if movie.Year != "" {
			fmt.Sscanf(movie.Year, "%d", &yearInt)
		}
		movieRecord := &database.Movie{
			Title:          movie.Title,
			Year:           yearInt,
			CanonicalPath:  movieDir,
			LibraryRoot:    libraryPath,
			Source:         "plex2jellyfin",
			SourcePriority: 100,
		}
		_, err := o.db.UpsertMovie(movieRecord)
		if err != nil {
			return &OrganizationResult{
				Success:       false,
				SourcePath:    sourcePath,
				TargetPath:    targetPath,
				BytesCopied:   result.BytesCopied,
				Duration:      result.Duration,
				Attempts:      result.Attempts,
				SourceQuality: sourceQuality,
				Error:         fmt.Errorf("DB upsert failed after file move — filesystem and database are inconsistent: %w", err),
			}, nil
		}
		if o.syncService != nil {
			o.db.SetMovieDirty(movieRecord.ID)
			o.syncService.QueueSync("movie", movieRecord.ID)
		}
	}

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

func (o *Organizer) OrganizeTVEpisode(sourcePath, libraryPath string) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil
	}

	return o.OrganizeTVWithParsed(sourcePath, libraryPath, *tv)
}

// OrganizeTVWithParsedAuto routes a parsed TV episode through the smart
// library selector before organizing. This ensures AI-enhanced moves
// land on the volume that already holds the series instead of always
// dropping onto the first configured TV library.
func (o *Organizer) OrganizeTVWithParsedAuto(sourcePath string, tv naming.TVShowInfo, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	size, err := getFileSize(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil
	}

	selection, err := o.selector.SelectTVShowLibrary(tv.Title, tv.Year, size)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to select library: %w", err),
		}, nil
	}

	log.Printf("[organizer] tv library selected (parsed): title=%q lib=%s reason=%s", tv.Title, selection.Library, selection.Reason)

	return o.OrganizeTVWithParsed(sourcePath, selection.Library, tv)
}

func (o *Organizer) OrganizeTVWithParsed(sourcePath, libraryPath string, tv naming.TVShowInfo) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	showDir := findExistingShowDir(libraryPath, tv.Title)
	reusingExistingShowDir := showDir != ""
	if showDir == "" {
		showName := naming.NormalizeMediaName(tv.Title, tv.Year)
		showDir = filepath.Join(libraryPath, showName)
	}
	if reusingExistingShowDir {
		yearInt := 0
		if tv.Year != "" {
			fmt.Sscanf(tv.Year, "%d", &yearInt)
		}
		decision := identity.CompareSeries(
			identity.SeriesIdentity{Title: tv.Title, Year: yearInt, Path: sourcePath},
			identity.SeriesIdentity{Path: showDir},
		)
		if decision.Verdict != identity.VerdictSame {
			reason := fmt.Sprintf("identity safety: %s", strings.Join(decision.Reasons, "; "))
			return &OrganizationResult{
				Success:    false,
				SourcePath: sourcePath,
				TargetPath: showDir,
				Skipped:    true,
				SkipReason: reason,
				Error:      fmt.Errorf("%s", reason),
			}, nil
		}
	}

	seasonDir := findExistingSeasonDir(showDir, tv.Season)
	if seasonDir == "" {
		seasonDir = filepath.Join(showDir, naming.FormatSeasonFolder(tv.Season))
	}
	ext := filepath.Ext(sourcePath)
	episodeName := naming.FormatTVEpisodeFilenameFromInfo(&tv, ext[1:])
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

	existingFile, existingFound := FindEpisodeFile(seasonDir, tv.Season, tv.Episode)
	var existingQuality *quality.QualityInfo
	if existingFound {
		existingQuality = quality.Parse(filepath.Base(existingFile))
	}

	if existingFound && !o.forceOverwrite {
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
		// same episode, source is better quality — fall through to overwrite
	}
	if !existingFound {
		existingFile = ""
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
			Error:         fmt.Errorf("unable to set show directory permissions: %w", err),
		}, nil
	}
	if err := o.applyDirOwnership(seasonDir); err != nil {
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to set season directory permissions: %w", err),
		}, nil
	}

	if o.dryRun {
		return &OrganizationResult{
			Success:         true,
			SourcePath:      sourcePath,
			TargetPath:      targetPath,
			SourceQuality:   sourceQuality,
			ExistingQuality: existingQuality,
		}, nil
	}

	opts := o.buildTransferOptions()

	var err error
	var result *transfer.TransferResult
	if o.keepSource {
		result, err = o.transferer.Copy(sourcePath, targetPath, opts)
	} else {
		result, err = o.transferer.Move(sourcePath, targetPath, opts)
	}

	if err != nil {
		if skipped := alreadyOrganizedResult(sourcePath, targetPath); skipped != nil {
			return skipped, nil
		}
		return &OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			BytesCopied:   result.BytesCopied,
			Duration:      result.Duration,
			Attempts:      result.Attempts,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("transfer failed: %w", err),
		}, nil
	}

	if existingFile != "" && existingFile != targetPath {
		if err := os.Remove(existingFile); err != nil && !os.IsNotExist(err) {
			log.Printf("[organizer] warning: failed to remove existing file %s: %v", existingFile, err)
		}
	}

	if o.db != nil {
		if mediaFile, err := o.db.GetMediaFile(sourcePath); err == nil && mediaFile != nil {
			mediaFile.Path = targetPath
			if err := o.db.UpdateMediaFile(mediaFile); err != nil {
				log.Printf("[organizer] warning: failed to update media file path after move: %v", err)
			}
		}
	}

	// HOLDEN Phase 3: Self-learning - update database with organized TV show
	if o.db != nil {
		yearInt := 0
		if tv.Year != "" {
			fmt.Sscanf(tv.Year, "%d", &yearInt)
		}
		seriesRecord := &database.Series{
			Title:          tv.Title,
			Year:           yearInt,
			CanonicalPath:  showDir,
			LibraryRoot:    libraryPath,
			Source:         "plex2jellyfin",
			SourcePriority: 100,
			EpisodeCount:   0,
		}
		_, err := o.db.UpsertSeries(seriesRecord)
		if err != nil {
			return &OrganizationResult{
				Success:       false,
				SourcePath:    sourcePath,
				TargetPath:    targetPath,
				BytesCopied:   result.BytesCopied,
				Duration:      result.Duration,
				Attempts:      result.Attempts,
				SourceQuality: sourceQuality,
				Error:         fmt.Errorf("DB upsert failed after file move — filesystem and database are inconsistent: %w", err),
			}, nil
		}
		if o.syncService != nil {
			o.db.SetSeriesDirty(seriesRecord.ID)
			o.syncService.QueueSync("series", seriesRecord.ID)
		}
	}

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

func (o *Organizer) OrganizeTVEpisodeAuto(sourcePath string, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	info, err := getFileSize(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil
	}

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil
	}

	selection, err := o.selector.SelectTVShowLibrary(tv.Title, tv.Year, info)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to select library: %w", err),
		}, nil
	}

	// Surface the selector's reasoning so "why did this land on STORAGE3
	// instead of STORAGE5" is answerable from the logs alone.
	log.Printf("[organizer] tv library selected: title=%q lib=%s reason=%s", tv.Title, selection.Library, selection.Reason)

	return o.OrganizeTVEpisode(sourcePath, selection.Library)
}

func (o *Organizer) OrganizeTVSeasonPackAuto(releaseDir string, getFileSize func(string) (int64, error)) (*SeasonPackResult, error) {
	releaseDir = filepath.Clean(releaseDir)
	packInfo, err := naming.ParseTVSeasonPackName(filepath.Base(releaseDir))
	if err != nil {
		return nil, err
	}

	videoFiles, err := collectVideoFiles(releaseDir)
	if err != nil {
		return nil, err
	}

	result := &SeasonPackResult{SourceDir: releaseDir}
	for _, path := range videoFiles {
		tv, err := naming.ParseTVShowFromPath(path)
		if err != nil || tv.Season <= 0 || tv.Episode <= 0 {
			result.Unresolved = append(result.Unresolved, path)
			continue
		}
		if tv.Title == "" {
			tv.Title = packInfo.Title
		}
		if tv.Year == "" {
			tv.Year = packInfo.Year
		}

		itemResult, err := o.OrganizeTVWithParsedAuto(path, *tv, getFileSize)
		if err != nil {
			result.Error = err
			return result, err
		}
		result.Imported = append(result.Imported, itemResult)
		if itemResult != nil {
			result.BytesCopied += itemResult.BytesCopied
			if itemResult.Error != nil && !itemResult.Skipped {
				result.Error = itemResult.Error
				result.SkipReason = "season_pack_partial_failure"
				if rollbackErr := rollbackSeasonPackImports(result.Imported); rollbackErr != nil {
					result.Error = fmt.Errorf("%w; rollback failed: %v", result.Error, rollbackErr)
				}
				result.Success = false
				return result, nil
			}
		}
	}

	if len(result.Imported) == 0 {
		if naming.IsExtrasRelease(filepath.Base(releaseDir)) {
			return o.organizeExtrasRelease(releaseDir, packInfo, result)
		}
		result.Skipped = true
		result.SkipReason = "season_pack_unresolved"
		result.Error = fmt.Errorf("season pack has no episode-numbered video files: %s", releaseDir)
		return result, nil
	}

	result.Success = result.Error == nil
	if !result.Success && result.SkipReason == "" {
		result.SkipReason = "season_pack_partial_failure"
	}
	return result, nil
}

// organizeExtrasRelease routes a bonus/extras release into the matching show's
// extras/ folder (Jellyfin's extras convention). It only matches existing show
// directories — an extras release for a show not present in any library is
// terminally skipped so it stops being retried.
func (o *Organizer) organizeExtrasRelease(releaseDir string, packInfo *naming.TVSeasonPackInfo, result *SeasonPackResult) (*SeasonPackResult, error) {
	showDir := ""
	for _, lib := range o.libraries {
		if dir := findExistingShowDir(lib, packInfo.Title); dir != "" {
			showDir = dir
			break
		}
	}
	if showDir == "" {
		result.Skipped = true
		result.SkipReason = "extras_unresolved"
		result.Error = fmt.Errorf("extras release has no matching show in any library: %s", releaseDir)
		return result, nil
	}

	videos := result.Unresolved
	result.Unresolved = nil
	if len(videos) == 0 {
		result.Skipped = true
		result.SkipReason = "extras_unresolved"
		result.Error = fmt.Errorf("extras release contains no video files: %s", releaseDir)
		return result, nil
	}

	extrasDir := filepath.Join(showDir, "extras")
	if !o.dryRun {
		if err := os.MkdirAll(extrasDir, 0755); err != nil {
			result.Error = fmt.Errorf("unable to create extras directory: %w", err)
			return result, nil
		}
		if err := o.applyDirOwnership(extrasDir); err != nil {
			result.Error = fmt.Errorf("unable to set extras directory permissions: %w", err)
			return result, nil
		}
	}

	opts := o.buildTransferOptions()
	for i, path := range videos {
		ext := filepath.Ext(path)
		cleaned := naming.CleanExtrasName(strings.TrimSuffix(filepath.Base(path), ext))
		if cleaned == "" {
			cleaned = strings.TrimSuffix(filepath.Base(path), ext)
		}
		targetPath := filepath.Join(extrasDir, cleaned+ext)
		item := &OrganizationResult{SourcePath: path, TargetPath: targetPath}
		if o.dryRun {
			item.Success = true
			result.Imported = append(result.Imported, item)
			continue
		}
		var transferResult *transfer.TransferResult
		var err error
		if o.keepSource {
			transferResult, err = o.transferer.Copy(path, targetPath, opts)
		} else {
			transferResult, err = o.transferer.Move(path, targetPath, opts)
		}
		if err != nil {
			result.Unresolved = append(result.Unresolved, videos[i:]...)
			result.Error = fmt.Errorf("extras transfer failed: %w", err)
			result.Success = false
			return result, nil
		}
		item.Success = true
		if transferResult != nil {
			item.BytesCopied = transferResult.BytesCopied
			result.BytesCopied += transferResult.BytesCopied
		}
		result.Imported = append(result.Imported, item)
	}

	result.Success = true
	return result, nil
}

func rollbackSeasonPackImports(imported []*OrganizationResult) error {
	var errs []error
	for i := len(imported) - 1; i >= 0; i-- {
		item := imported[i]
		if item == nil || !item.Success || item.SourcePath == "" || item.TargetPath == "" {
			continue
		}
		if _, err := os.Stat(item.TargetPath); err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("stat target %s: %w", item.TargetPath, err))
			}
			continue
		}
		if _, err := os.Stat(item.SourcePath); err == nil {
			errs = append(errs, fmt.Errorf("source already exists, not overwriting: %s", item.SourcePath))
			continue
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("stat source %s: %w", item.SourcePath, err))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.SourcePath), 0755); err != nil {
			errs = append(errs, fmt.Errorf("create rollback dir %s: %w", filepath.Dir(item.SourcePath), err))
			continue
		}
		if err := os.Rename(item.TargetPath, item.SourcePath); err != nil {
			errs = append(errs, fmt.Errorf("rollback %s -> %s: %w", item.TargetPath, item.SourcePath, err))
		}
	}
	return errors.Join(errs...)
}

func collectVideoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if video.IsVideo(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// findExistingMediaFile scans a directory for existing video files and returns
// the path and quality info of the best existing file.
// Returns ("", nil) if no video files found.
func (o *Organizer) findExistingMediaFile(dir string) (string, *quality.QualityInfo) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}

	var bestPath string
	var bestQuality *quality.QualityInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !video.IsVideo(entry.Name()) {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		fileQuality := quality.Parse(entry.Name())

		if bestQuality == nil || fileQuality.IsBetterThan(bestQuality) {
			bestPath = filePath
			bestQuality = fileQuality
		}
	}

	return bestPath, bestQuality
}

func findExistingShowDir(libraryPath, showTitle string) string {
	entries, err := os.ReadDir(libraryPath)
	if err != nil {
		return ""
	}

	normalizedTitle := database.NormalizeForMatch(showTitle)
	yearPattern := regexp.MustCompile(`\s*\(\d{4}\)\s*$`)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()

		// Try exact case-insensitive match first (fast path)
		if strings.EqualFold(dirName, showTitle) {
			return filepath.Join(libraryPath, dirName)
		}

		// Strip year from directory name, then normalize and compare
		baseName := yearPattern.ReplaceAllString(dirName, "")
		if database.NormalizeForMatch(baseName) == normalizedTitle {
			return filepath.Join(libraryPath, dirName)
		}
	}

	return ""
}

type FolderOrganizationResult struct {
	Analysis        *analyzer.FolderAnalysis
	MediaResult     *OrganizationResult
	SubtitlesCopied []string
	JunkRemoved     []string
	SamplesRemoved  []string
	ExtrasSkipped   []string
	Error           error
}

func (o *Organizer) OrganizeFolder(folderPath, libraryPath string, keepExtras bool) (*FolderOrganizationResult, error) {
	analysis, err := analyzer.AnalyzeFolder(folderPath)
	if err != nil {
		return &FolderOrganizationResult{Error: err}, err
	}

	result := &FolderOrganizationResult{
		Analysis: analysis,
	}

	if !analysis.HasUsableMedia() {
		result.Error = fmt.Errorf("no usable media found in folder (incomplete or empty)")
		return result, nil
	}

	switch analysis.MediaType {
	case analyzer.MediaTypeMovie:
		mediaResult, err := o.OrganizeMovie(analysis.MainMediaFile.Path, libraryPath)
		result.MediaResult = mediaResult
		if err != nil || !mediaResult.Success {
			return result, err
		}

		if !o.dryRun {
			result.SubtitlesCopied = o.copySubtitles(analysis, filepath.Dir(mediaResult.TargetPath))
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
			if !keepExtras {
				result.ExtrasSkipped = o.getFilePaths(analysis.ExtraFiles)
			}
		}

	case analyzer.MediaTypeTVEpisode:
		mediaResult, err := o.OrganizeTVEpisode(analysis.MainMediaFile.Path, libraryPath)
		result.MediaResult = mediaResult
		if err != nil || !mediaResult.Success {
			return result, err
		}

		if !o.dryRun {
			result.SubtitlesCopied = o.copySubtitles(analysis, filepath.Dir(mediaResult.TargetPath))
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
		}

	case analyzer.MediaTypeTVSeason:
		for _, mediaFile := range analysis.MediaFiles {
			mediaResult, err := o.OrganizeTVEpisode(mediaFile.Path, libraryPath)
			if err != nil {
				result.Error = err
				return result, err
			}
			result.MediaResult = mediaResult
		}

		if !o.dryRun {
			result.JunkRemoved = o.removeFiles(analysis.JunkFiles)
			result.SamplesRemoved = o.removeFiles(analysis.SampleFiles)
		}

	default:
		result.Error = fmt.Errorf("unknown media type")
		return result, nil
	}

	return result, nil
}

// languageSuffixes lists subtitle language/flag suffixes to strip when matching
// subtitle stems against the video stem.
var languageSuffixes = []string{
	".forced", ".sdh", ".hi",
	".eng", ".en", ".fra", ".fre", ".ger", ".deu", ".spa", ".por", ".ita",
	".rus", ".pol", ".chi", ".jpn", ".kor", ".ara", ".ces", ".cze", ".nld",
	".dut", ".swe", ".nor", ".dan", ".fin", ".tur", ".hun", ".heb", ".ron",
}

// subtitleMatchesVideo reports whether a subtitle file (identified by its base
// name without extension) corresponds to the given video stem. One language or
// flag suffix is stripped before the case-insensitive comparison.
func subtitleMatchesVideo(subName, videoStem string) bool {
	subExt := strings.ToLower(filepath.Ext(subName))
	_ = subExt
	subStem := subName[:len(subName)-len(filepath.Ext(subName))]
	lower := strings.ToLower(subStem)
	videoLower := strings.ToLower(videoStem)

	if lower == videoLower {
		return true
	}
	for _, suf := range languageSuffixes {
		if strings.HasSuffix(lower, suf) {
			trimmed := lower[:len(lower)-len(suf)]
			if trimmed == videoLower {
				return true
			}
		}
	}
	return false
}

func (o *Organizer) copySubtitles(analysis *analyzer.FolderAnalysis, targetDir string) []string {
	if analysis.MainMediaFile == nil {
		return nil
	}
	videoName := analysis.MainMediaFile.Name
	videoStem := videoName[:len(videoName)-len(filepath.Ext(videoName))]

	var copied []string
	for _, sub := range analysis.SubtitleFiles {
		if !subtitleMatchesVideo(sub.Name, videoStem) {
			continue
		}
		targetPath := filepath.Join(targetDir, sub.Name)
		opts := transfer.TransferOptions{
			Timeout:       30 * time.Second,
			RetryAttempts: 2,
			TargetUID:     o.targetUID,
			TargetGID:     o.targetGID,
			FileMode:      o.fileMode,
		}
		_, err := o.transferer.Copy(sub.Path, targetPath, opts)
		if err == nil {
			copied = append(copied, sub.Name)
		} else {
			log.Printf("[organizer] subtitle copy failed: src=%s dst=%s err=%v", sub.Path, targetPath, err)
		}
	}
	return copied
}

func (o *Organizer) removeFiles(files []analyzer.FileInfo) []string {
	var removed []string
	for _, f := range files {
		if err := os.Remove(f.Path); err == nil {
			removed = append(removed, f.Name)
		}
	}
	return removed
}

func (o *Organizer) getFilePaths(files []analyzer.FileInfo) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Name
	}
	return paths
}

// findExistingSeasonDir looks for an existing season directory in the show folder,
// matching both padded ("Season 01") and non-padded ("Season 1") formats.
// Returns the full path to the existing directory, or empty string if none found.
func findExistingSeasonDir(showDir string, season int) string {
	entries, err := os.ReadDir(showDir)
	if err != nil {
		return ""
	}

	seasonPrefix := "season "

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		dirLower := strings.ToLower(dirName)

		if !strings.HasPrefix(dirLower, seasonPrefix) {
			continue
		}

		numStr := strings.TrimSpace(dirLower[len(seasonPrefix):])
		var num int
		if _, err := fmt.Sscanf(numStr, "%d", &num); err == nil && num == season {
			return filepath.Join(showDir, dirName)
		}
	}

	return ""
}
