package organizer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/analyzer"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
	"github.com/Nomadcxx/plex2jellyfin/internal/identity"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/library"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
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
	// InventoryError records post-publication inventory or replacement cleanup
	// failures. The new target is already on disk, so Success stays true and
	// TargetPath is set; callers can surface the consistency issue separately.
	InventoryError    error
	sourceRetained    bool
	publishedIdentity string
	replacedExisting  bool
}

// OrganizationFinalizer records an organize result before its organization
// lock is released, when one has been acquired. It must not organize the same target.
type OrganizationFinalizer func(*OrganizationResult, error)

type organizationTargetLock struct {
	mu   sync.Mutex
	refs int
}

// Active entries are removed on unlock so a long-running daemon does not keep
// one mutex per title or episode it has ever seen.
var organizationTargetLocks = struct {
	sync.Mutex
	locks map[string]*organizationTargetLock
}{locks: make(map[string]*organizationTargetLock)}

func organizationTargetLockKey(targetPath string) string {
	key, err := filepath.Abs(targetPath)
	if err != nil {
		key = filepath.Clean(targetPath)
	}
	return key
}

func lockOrganizationKey(key string) (func(), error) {
	organizationTargetLocks.Lock()
	lock := organizationTargetLocks.locks[key]
	if lock == nil {
		lock = &organizationTargetLock{}
		organizationTargetLocks.locks[key] = lock
	}
	lock.refs++
	organizationTargetLocks.Unlock()

	lock.mu.Lock()
	file, err := acquireOrganizationFileLock(key)
	if err != nil {
		lock.mu.Unlock()
		organizationTargetLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(organizationTargetLocks.locks, key)
		}
		organizationTargetLocks.Unlock()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		lock.mu.Unlock()
		organizationTargetLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(organizationTargetLocks.locks, key)
		}
		organizationTargetLocks.Unlock()
	}, nil
}

func acquireOrganizationFileLock(key string) (*os.File, error) {
	configDir, err := paths.Plex2JellyfinDir()
	if err != nil {
		return nil, fmt.Errorf("resolve organizer lock directory: %w", err)
	}
	lockDir := filepath.Join(configDir, "organizer-locks")
	configParent := filepath.Dir(configDir)
	for _, dir := range []string{configParent, configDir, lockDir} {
		info, err := os.Lstat(dir)
		if err == nil && !info.IsDir() {
			return nil, fmt.Errorf("organizer lock path %q is not a directory", dir)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect organizer lock path %q: %w", dir, err)
		}
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("create organizer config directory: %w", err)
	}
	info, err := os.Lstat(configDir)
	if err != nil {
		return nil, fmt.Errorf("inspect organizer config directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("organizer config path %q is not a directory", configDir)
	}
	if err := paths.ChownToActualUser(configDir); err != nil {
		return nil, fmt.Errorf("set organizer config directory owner: %w", err)
	}
	if err := os.Mkdir(lockDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create organizer lock directory: %w", err)
	}
	info, err = os.Lstat(lockDir)
	if err != nil {
		return nil, fmt.Errorf("inspect organizer lock directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("organizer lock path %q is not a directory", lockDir)
	}
	if err := os.Chmod(lockDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure organizer lock directory: %w", err)
	}
	if err := paths.ChownToActualUser(lockDir); err != nil {
		return nil, fmt.Errorf("set organizer lock directory owner: %w", err)
	}

	hash := sha256.Sum256([]byte(key))
	path := filepath.Join(lockDir, hex.EncodeToString(hash[:])+".lock")
	// Keep lock files after release so waiters continue to use the same inode.
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o444)
	if err != nil {
		return nil, fmt.Errorf("open organizer lock file: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect organizer lock file: %w", err)
	}
	if !fileInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("organizer lock path %q is not a regular file", path)
	}
	// Read-only descriptors can hold flock; the private directory limits access to the config owner.
	if err := file.Chmod(0o444); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set organizer lock file permissions: %w", err)
	}
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("acquire organizer file lock: %w", err)
		}
		break
	}
	return file, nil
}

// lockOrganizationKeys takes multiple season locks in stable order to avoid deadlocks.
func lockOrganizationKeys(keys []string) (func(), error) {
	sort.Strings(keys)
	unlock := make([]func(), 0, len(keys))
	for i, key := range keys {
		if i > 0 && key == keys[i-1] {
			continue
		}
		release, err := lockOrganizationKey(key)
		if err != nil {
			for j := len(unlock) - 1; j >= 0; j-- {
				unlock[j]()
			}
			return nil, fmt.Errorf("lock organizer key %q: %w", key, err)
		}
		unlock = append(unlock, release)
	}
	return func() {
		for i := len(unlock) - 1; i >= 0; i-- {
			unlock[i]()
		}
	}, nil
}

func organizeWithTargetLock(targetPath string, finalize OrganizationFinalizer, organize func() (*OrganizationResult, error)) (*OrganizationResult, error) {
	return organizeWithKeyLock(organizationTargetLockKey(targetPath), finalize, organize)
}

func organizeWithKeyLock(key string, finalize OrganizationFinalizer, organize func() (*OrganizationResult, error)) (*OrganizationResult, error) {
	unlock, err := lockOrganizationKey(key)
	if err != nil {
		lockErr := fmt.Errorf("acquire organizer lock: %w", err)
		if finalize != nil {
			finalize(nil, lockErr)
		}
		return nil, lockErr
	}
	defer unlock()

	result, err := organize()
	if finalize != nil {
		finalize(result, err)
	}
	return result, err
}

func finalizeOrganization(result *OrganizationResult, err error, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	if finalize != nil {
		finalize(result, err)
	}
	return result, err
}

type SeasonPackResult struct {
	Success   bool
	SourceDir string
	Imported  []*OrganizationResult
	// FailedEpisode retains the item that aborted the pack after earlier imports rolled back.
	FailedEpisode *OrganizationResult
	Unresolved    []string
	Skipped       bool
	SkipReason    string
	Error         error
	BytesCopied   int64
}

// SeasonPackFinalizer records a pack outcome before its show-season locks release.
type SeasonPackFinalizer func(*SeasonPackResult, error)

type SourceTransferMode uint8

const (
	TransferMove SourceTransferMode = iota
	TransferCopy
	TransferSeed
)

type TransferModeResolver func(sourcePath string) SourceTransferMode

type Organizer struct {
	dryRun               bool
	keepSource           bool
	forceOverwrite       bool
	libraries            []string
	selector             *library.Selector
	transferer           transfer.Transferer
	seedTransferer       transfer.Transferer
	transferModeResolver TransferModeResolver
	timeout              time.Duration
	checksumVerify       bool
	targetUID            int
	targetGID            int
	fileMode             os.FileMode
	dirMode              os.FileMode
	sonarrClient         *sonarr.Client
	jellyfinClient       *jellyfin.Client
	playbackSafety       bool
	db                   *database.MediaDB
	syncService          *syncsvc.SyncService
	pathIndexer          PathIndexer
	pluginClient         *jellyfin.PluginClient
	pauseOnScan          bool
	paused               bool
	pauseMu              sync.RWMutex
	playbackLocks        *jellyfin.PlaybackLockManager
	deferredQueue        *jellyfin.DeferredQueue
}

// PathIndexer indexes a destination path into media_files after a successful transfer.
// Prefer injecting FileScanner.ScanPath (or a thin wrapper) to avoid organizer↔scanner cycles.
type PathIndexer func(ctx context.Context, path, libraryRoot, mediaType string) error

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
	org.seedTransferer = transfer.NewHardlinkCopyTransferer(org.transferer)

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

func WithTransferModeResolver(resolve TransferModeResolver) func(*Organizer) {
	return func(o *Organizer) {
		o.transferModeResolver = resolve
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

// WithPathIndexer injects post-move media_files indexing (typically FileScanner.ScanPath).
func WithPathIndexer(index PathIndexer) func(*Organizer) {
	return func(o *Organizer) {
		o.pathIndexer = index
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

// alreadyPublished reports whether existing is already this source's published
// copy. A shared inode identifies hardlinked seed imports. For copied sources,
// use the successful decision's source path and metadata; size alone cannot
// distinguish a repeated import from a same-sized quality upgrade.
func (o *Organizer) alreadyPublished(sourcePath, existingPath string) bool {
	src, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}
	dst, err := os.Stat(existingPath)
	if err != nil {
		return false
	}
	if os.SameFile(src, dst) {
		return true
	}
	if o.forceOverwrite || o.db == nil {
		return false
	}
	decision, err := o.db.GetDecisionByTargetPath(existingPath)
	return err == nil && decision != nil &&
		filepath.Clean(decision.SourcePath) == filepath.Clean(sourcePath) &&
		decision.SourceSize == src.Size() && decision.SourceModifiedAt != nil &&
		decision.SourceModifiedAt.Equal(src.ModTime())
}

func hasQualityEvidence(q *quality.QualityInfo) bool {
	return q != nil && (q.Source != quality.SourceUnknown ||
		q.Resolution != quality.ResolutionUnknown ||
		q.HDR != quality.HDRNone ||
		q.Audio != quality.AudioUnknown ||
		q.Is3D || q.IsProper || q.IsExtended)
}

func (o *Organizer) qualityForExistingTarget(targetPath string) (*quality.QualityInfo, error) {
	existingQuality := quality.Parse(filepath.Base(targetPath))
	if hasQualityEvidence(existingQuality) {
		return existingQuality, nil
	}
	if o.db == nil {
		return nil, nil
	}

	decision, err := o.db.GetDecisionByTargetPath(targetPath)
	if err != nil {
		return nil, err
	}
	if decision == nil || strings.TrimSpace(decision.SourceFilename) == "" {
		return nil, nil
	}

	existingQuality = quality.Parse(decision.SourceFilename)
	if !hasQualityEvidence(existingQuality) {
		return nil, nil
	}
	return existingQuality, nil
}

func qualityReplacementSkipReason(sourceQuality, existingQuality *quality.QualityInfo) string {
	if !hasQualityEvidence(sourceQuality) {
		return "incoming file quality evidence is unavailable"
	}
	if !hasQualityEvidence(existingQuality) {
		return "existing file quality evidence is unavailable; verify the existing copy, then rerun organize --force to replace it"
	}
	if !sourceQuality.IsBetterThan(existingQuality) {
		return fmt.Sprintf("existing file has equal or better quality (%s vs %s)", existingQuality.String(), sourceQuality.String())
	}
	return ""
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

func (o *Organizer) transferSource(sourcePath, targetPath string, opts transfer.TransferOptions, canonicalPath, libraryRoot string, replaceDestination bool) (*transfer.TransferResult, bool, error) {
	if err := o.waitIfScanning(o.timeout); err != nil {
		return &transfer.TransferResult{Error: err}, false, err
	}

	mode := TransferMove
	if o.transferModeResolver != nil {
		mode = o.transferModeResolver(sourcePath)
	} else if o.keepSource {
		mode = TransferCopy
	}

	var result *transfer.TransferResult
	var err error
	switch mode {
	case TransferMove:
		result, err = o.moveTrackedSource(sourcePath, targetPath, opts, canonicalPath, libraryRoot, libraryRoot != "", "", replaceDestination)
	case TransferCopy:
		if replaceDestination {
			result, err = o.replaceWithRetainedCopy(sourcePath, targetPath, opts, o.transferer.Copy)
		} else {
			result, err = o.transferer.Copy(sourcePath, targetPath, opts)
		}
	case TransferSeed:
		if replaceDestination {
			result, err = o.replaceWithRetainedCopy(sourcePath, targetPath, opts, o.seedTransferer.Copy)
		} else {
			result, err = o.seedTransferer.Copy(sourcePath, targetPath, opts)
		}
	default:
		err := fmt.Errorf("unsupported source transfer mode %d", mode)
		return &transfer.TransferResult{Error: err}, false, err
	}
	if err == nil {
		err = result.Validate()
	}
	if result == nil {
		result = &transfer.TransferResult{Error: err}
	}
	return result, mode != TransferMove, err
}

func (o *Organizer) replaceWithRetainedCopy(sourcePath, targetPath string, opts transfer.TransferOptions, copyFile func(string, string, transfer.TransferOptions) (*transfer.TransferResult, error)) (result *transfer.TransferResult, err error) {
	sourceIdentity, ok := fileidentity.FromLstat(sourcePath)
	if !ok {
		return nil, fmt.Errorf("source has no stable identity: %s", sourcePath)
	}
	destinationInfo, err := os.Lstat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("inspect replacement destination %s: %w", targetPath, err)
	}
	contentInfo, err := os.Stat(targetPath)
	if err != nil || !contentInfo.Mode().IsRegular() {
		if err == nil {
			err = fmt.Errorf("destination is not a regular file")
		}
		return nil, fmt.Errorf("inspect replacement destination %s: %w", targetPath, err)
	}
	destinationIdentity, ok := fileidentity.FromInfo(destinationInfo)
	if !ok {
		return nil, fmt.Errorf("replacement destination has no stable identity: %s", targetPath)
	}
	stagePath, err := fileidentity.NewQuarantinePath(targetPath)
	if err != nil {
		return nil, err
	}
	var stageIdentity string
	defer func() {
		if stageIdentity != "" && fileidentity.MatchesLstat(stagePath, stageIdentity) {
			if cleanupErr := fileidentity.RemoveIfMatches(stagePath, stageIdentity, true); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("remove staged replacement %s: %w", stagePath, cleanupErr))
			}
		}
	}()
	result, err = copyFile(sourcePath, stagePath, opts)
	if err != nil {
		return result, err
	}
	if err := result.Validate(); err != nil {
		return result, err
	}
	stageInfo, err := os.Lstat(stagePath)
	if err != nil {
		return result, fmt.Errorf("inspect staged replacement %s: %w", stagePath, err)
	}
	if !stageInfo.Mode().IsRegular() {
		return result, fmt.Errorf("staged replacement is not a regular file: %s", stagePath)
	}
	stageIdentity, ok = fileidentity.FromInfo(stageInfo)
	if !ok {
		return result, fmt.Errorf("staged replacement has no stable identity: %s", stagePath)
	}
	if !fileidentity.MatchesLstat(sourcePath, sourceIdentity) {
		sourceInfo, sourceErr := os.Stat(sourcePath)
		stagedContentInfo, stageErr := os.Stat(stagePath)
		if sourceErr != nil || stageErr != nil || !os.SameFile(sourceInfo, stagedContentInfo) {
			return result, fmt.Errorf("source changed while preparing replacement: %s", sourcePath)
		}
	}
	if !fileidentity.MatchesLstat(targetPath, destinationIdentity) {
		return result, fmt.Errorf("replacement destination changed while preparing copy: %s", targetPath)
	}
	if err := replaceStagedFile(stagePath, targetPath, stageIdentity, destinationIdentity); err != nil {
		return result, err
	}
	result.Published = true
	result.PublishedIdentity = stageIdentity
	result.SourceRemoved = false
	return result, nil
}

func replaceStagedFile(stagePath, targetPath, stageIdentity, targetIdentity string) error {
	if err := fileidentity.Exchange(stagePath, targetPath); err != nil {
		return err
	}
	if !fileidentity.MatchesLstat(targetPath, stageIdentity) || !fileidentity.MatchesLstat(stagePath, targetIdentity) {
		var rollbackErr error
		if fileidentity.MatchesLstat(targetPath, stageIdentity) {
			rollbackErr = fileidentity.Exchange(stagePath, targetPath)
		}
		return errors.Join(fmt.Errorf("file identities changed during destination exchange"), rollbackErr)
	}
	if err := fileidentity.RemoveIfMatches(stagePath, targetIdentity, true); err != nil {
		var rollbackErr error
		if fileidentity.MatchesLstat(targetPath, stageIdentity) && fileidentity.MatchesLstat(stagePath, targetIdentity) {
			rollbackErr = fileidentity.Exchange(stagePath, targetPath)
		}
		return errors.Join(fmt.Errorf("remove replaced destination: %w", err), rollbackErr)
	}
	return nil
}

func (o *Organizer) moveTrackedSource(sourcePath, targetPath string, opts transfer.TransferOptions, canonicalPath, libraryRoot string, setLibraryRoot bool, expectedIdentity string, replaceDestination bool) (*transfer.TransferResult, error) {
	var file *database.MediaFile
	if o.db != nil {
		var err error
		file, err = o.db.GetMediaFile(sourcePath)
		if err != nil {
			return &transfer.TransferResult{Error: err}, err
		}
	}
	if file != nil {
		targetFile := *file
		targetFile.Path = targetPath
		if setLibraryRoot {
			targetFile.LibraryRoot = libraryRoot
		}
		var result *transfer.TransferResult
		move := func(sourceIdentity, sourceQuarantinePath string, recordPublished func(string, string) error) (database.MediaFileMoveAttempt, error) {
			if expectedIdentity != "" && sourceIdentity != expectedIdentity {
				return database.MediaFileMoveAttempt{}, fmt.Errorf("source changed before move: %s", sourcePath)
			}
			opts.MoveSourceIdentity = sourceIdentity
			opts.MoveSourceQuarantinePath = sourceQuarantinePath
			opts.OnPublished = recordPublished
			var moveErr error
			result, moveErr = o.transferer.Move(sourcePath, targetPath, opts)
			attempt := database.MediaFileMoveAttempt{}
			if result != nil {
				attempt = database.MediaFileMoveAttempt{
					Published:                 result.Published,
					SourceRemoved:             result.SourceRemoved,
					PublishedIdentity:         result.PublishedIdentity,
					DestinationQuarantinePath: result.DestinationQuarantinePath,
					PendingSourceRemoval:      result.PendingSourceRemoval,
				}
			}
			if moveErr == nil {
				moveErr = result.Validate()
			}
			return attempt, moveErr
		}
		var err error
		if replaceDestination {
			err = o.db.MoveMediaFileWithReplacement(&targetFile, sourcePath, canonicalPath, false, move)
		} else {
			err = o.db.MoveMediaFileWith(&targetFile, sourcePath, canonicalPath, false, move)
		}
		if err != nil {
			if result == nil {
				result = &transfer.TransferResult{}
			}
			result.Error = err
			return result, err
		}
		return result, nil
	}

	if expectedIdentity != "" {
		opts.MoveSourceIdentity = expectedIdentity
	}
	result, err := o.transferer.Move(sourcePath, targetPath, opts)
	if err == nil {
		err = result.Validate()
	}
	if result == nil {
		result = &transfer.TransferResult{Error: err}
	}
	return result, err
}

func publishedMediaIdentity(result *transfer.TransferResult, path string) string {
	if result != nil && result.PublishedIdentity != "" {
		return result.PublishedIdentity
	}
	identity, _ := fileidentity.FromLstat(path)
	return identity
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

// inventoryIndexTimeout bounds a single post-move ScanPath so a stalled
// filesystem or database cannot wedge the organize pipeline indefinitely.
const inventoryIndexTimeout = 2 * time.Minute

// refreshInventoryAfterTransfer indexes the destination via PathIndexer and then
// removes the stale source/replaced media_files rows it supersedes. Returns a
// consistency error the caller reports alongside an otherwise successful transfer.
func (o *Organizer) refreshInventoryAfterTransfer(sourcePath, targetPath string, replacedPaths []string, libraryRoot, canonicalPath, mediaType string, sourceRetained bool) error {
	if o.db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), inventoryIndexTimeout)
	defer cancel()
	if o.pathIndexer == nil {
		// Legacy fallback when callers have not wired ScanPath yet.
		sourceFile, err := o.db.GetMediaFile(sourcePath)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read source inventory row: %w", err)
		}
		destinationFile, err := o.db.GetMediaFile(targetPath)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read destination inventory row: %w", err)
		}
		if sourceFile != nil && destinationFile == nil {
			sourceFile.Path = targetPath
			if err := o.db.UpdateMediaFile(sourceFile); err != nil {
				return fmt.Errorf("update source inventory path: %w", err)
			}
			destinationFile = sourceFile
		}
		if len(replacedPaths) > 0 && destinationFile == nil {
			return fmt.Errorf("keep replaced inventory rows: destination was not indexed")
		}
	} else {
		// Index the destination before dropping the rows it replaces. If indexing
		// fails, the stale rows are the only remaining record of the file, so
		// deleting them first would leave the moved file absent from media_files
		// entirely — invisible to the sweep, verification, and duplicate analysis.
		if err := o.pathIndexer(ctx, targetPath, libraryRoot, mediaType); err != nil {
			return fmt.Errorf("index destination: %w", err)
		}
	}

	var errs []error
	if sourceRetained {
		if err := o.db.RetireMediaFileAfterCopy(sourcePath, targetPath, canonicalPath, libraryRoot); err != nil {
			errs = append(errs, fmt.Errorf("merge retained source inventory row: %w", err))
		}
	} else if o.pathIndexer != nil {
		sourceRole := "stale"
		if err := o.db.DeleteMediaFile(sourcePath); err != nil {
			errs = append(errs, fmt.Errorf("remove %s source row: %w", sourceRole, err))
		}
	}
	for _, replacedPath := range replacedPaths {
		if replacedPath == "" || replacedPath == targetPath || replacedPath == sourcePath {
			continue
		}
		if err := o.db.DeleteMediaFile(replacedPath); err != nil {
			errs = append(errs, fmt.Errorf("remove stale replaced row %s: %w", replacedPath, err))
		}
	}
	return errors.Join(errs...)
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

func (o *Organizer) waitIfScanning(timeout time.Duration) error {
	if !o.pauseOnScan {
		return nil
	}
	if o.pluginClient == nil {
		return fmt.Errorf("Jellyfin scan coordination is enabled without a plugin client")
	}

	deadline := time.Now().Add(timeout)
	for {
		scans, err := o.pluginClient.GetActiveScans()
		if err != nil {
			return fmt.Errorf("checking Jellyfin scan status: %w", err)
		}
		if scans == nil || len(scans.Scans) == 0 {
			return nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("timeout waiting for Jellyfin scan to complete")
		}
		if remaining > 5*time.Second {
			remaining = 5 * time.Second
		}
		time.Sleep(remaining)
	}
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
	return o.OrganizeMovieFinalized(sourcePath, libraryPath, nil)
}

// OrganizeMovieFinalized invokes finalize before releasing the target lock when
// parsing succeeds.
func (o *Organizer) OrganizeMovieFinalized(sourcePath, libraryPath string, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	movie, err := naming.ParseMovieFromPath(sourcePath)
	if err != nil {
		return finalizeOrganization(&OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse movie name: %w", err),
		}, nil, finalize)
	}

	return o.OrganizeMovieWithParsedFinalized(sourcePath, libraryPath, *movie, finalize)
}

func (o *Organizer) OrganizeMovieWithParsed(sourcePath, libraryPath string, movie naming.MovieInfo) (*OrganizationResult, error) {
	return o.OrganizeMovieWithParsedFinalized(sourcePath, libraryPath, movie, nil)
}

// OrganizeMovieWithParsedFinalized holds the movie target lock through finalize.
func (o *Organizer) OrganizeMovieWithParsedFinalized(sourcePath, libraryPath string, movie naming.MovieInfo, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	cleanName := naming.NormalizeMediaName(movie.Title, movie.Year)
	movieDir := filepath.Join(libraryPath, cleanName)
	return organizeWithTargetLock(movieDir, finalize, func() (*OrganizationResult, error) {
		return o.organizeMovieWithParsed(sourcePath, libraryPath, movie)
	})
}

func (o *Organizer) organizeMovieWithParsed(sourcePath, libraryPath string, movie naming.MovieInfo) (*OrganizationResult, error) {
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
	if existingFile != "" {
		if skipped := alreadyOrganizedResult(sourcePath, targetPath); skipped != nil {
			return skipped, nil
		}
	}
	if existingFile != "" && o.alreadyPublished(sourcePath, existingFile) {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			TargetPath: existingFile,
			Skipped:    true,
			SkipReason: "already imported from this source",
		}, nil
	}
	if existingFile != "" && !o.forceOverwrite {
		qualityInfo, err := o.qualityForExistingTarget(existingFile)
		if err != nil {
			return &OrganizationResult{
				SourcePath:    sourcePath,
				TargetPath:    existingFile,
				Skipped:       true,
				SkipReason:    fmt.Sprintf("could not read existing quality evidence: %v", err),
				SourceQuality: sourceQuality,
			}, nil
		}
		existingQuality = qualityInfo
		if reason := qualityReplacementSkipReason(sourceQuality, existingQuality); reason != "" {
			return &OrganizationResult{
				Success:         false,
				SourcePath:      sourcePath,
				TargetPath:      existingFile,
				Skipped:         true,
				SkipReason:      reason,
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

	result, sourceRetained, err := o.transferSource(sourcePath, targetPath, opts, movieDir, libraryPath, existingFile == targetPath)

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

	var replacedPaths []string
	var replacementErr error
	if existingFile != "" && existingFile != targetPath {
		if err := os.Remove(existingFile); err != nil && !os.IsNotExist(err) {
			replacementErr = fmt.Errorf("remove replaced file %s: %w", existingFile, err)
		} else {
			replacedPaths = append(replacedPaths, existingFile)
		}
	}

	// The bytes have already landed at targetPath. An indexing failure is a
	// consistency problem to report, not a failed organize: reporting failure
	// here would drop target_path from the decision and hide a file that is
	// genuinely on disk.
	inventoryErr := errors.Join(replacementErr, o.refreshInventoryAfterTransfer(sourcePath, targetPath, replacedPaths, libraryPath, movieDir, "movie", sourceRetained))

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
				Success:           false,
				SourcePath:        sourcePath,
				TargetPath:        targetPath,
				BytesCopied:       result.BytesCopied,
				Duration:          result.Duration,
				Attempts:          result.Attempts,
				SourceQuality:     sourceQuality,
				sourceRetained:    sourceRetained,
				publishedIdentity: publishedMediaIdentity(result, targetPath),
				Error:             fmt.Errorf("DB upsert failed after file move — filesystem and database are inconsistent: %w", err),
			}, nil
		}
		if o.syncService != nil {
			o.db.SetMovieDirty(movieRecord.ID)
			o.syncService.QueueSync("movie", movieRecord.ID)
		}
	}

	return &OrganizationResult{
		Success:           true,
		SourcePath:        sourcePath,
		TargetPath:        targetPath,
		BytesCopied:       result.BytesCopied,
		Duration:          result.Duration,
		Attempts:          result.Attempts,
		SourceQuality:     sourceQuality,
		ExistingQuality:   existingQuality,
		InventoryError:    inventoryErr,
		sourceRetained:    sourceRetained,
		publishedIdentity: publishedMediaIdentity(result, targetPath),
	}, nil
}

func (o *Organizer) OrganizeTVEpisode(sourcePath, libraryPath string) (*OrganizationResult, error) {
	return o.OrganizeTVEpisodeFinalized(sourcePath, libraryPath, nil)
}

// OrganizeTVEpisodeFinalized invokes finalize before releasing the show-season
// lock when parsing succeeds.
func (o *Organizer) OrganizeTVEpisodeFinalized(sourcePath, libraryPath string, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return finalizeOrganization(&OrganizationResult{
			Success:       false,
			SourcePath:    sourcePath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil, finalize)
	}

	return o.OrganizeTVWithParsedFinalized(sourcePath, libraryPath, *tv, finalize)
}

// OrganizeTVWithParsedAuto routes a parsed TV episode through the smart
// library selector before organizing. This ensures AI-enhanced moves
// land on the volume that already holds the series instead of always
// dropping onto the first configured TV library.
func (o *Organizer) OrganizeTVWithParsedAuto(sourcePath string, tv naming.TVShowInfo, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	return o.OrganizeTVWithParsedAutoFinalized(sourcePath, tv, getFileSize, nil)
}

// OrganizeTVWithParsedAutoFinalized selects and organizes under the show-season
// lock so concurrent imports cannot choose different libraries for one episode.
func (o *Organizer) OrganizeTVWithParsedAutoFinalized(sourcePath string, tv naming.TVShowInfo, getFileSize func(string) (int64, error), finalize OrganizationFinalizer) (*OrganizationResult, error) {
	size, err := getFileSize(sourcePath)
	if err != nil {
		return finalizeOrganization(&OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil, finalize)
	}

	return o.organizeTVWithParsedAutoFinalized(sourcePath, tv, size, finalize)
}

func (o *Organizer) organizeTVWithParsedAutoFinalized(sourcePath string, tv naming.TVShowInfo, size int64, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	return organizeWithKeyLock(tvSeasonOrganizationLockKey(tv), finalize, func() (*OrganizationResult, error) {
		return o.organizeTVWithParsedAuto(sourcePath, tv, size)
	})
}

func (o *Organizer) organizeTVWithParsedAuto(sourcePath string, tv naming.TVShowInfo, size int64) (*OrganizationResult, error) {
	selection, err := o.selector.SelectTVShowLibrary(tv.Title, tv.Year, size)
	if err != nil {
		return &OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to select library: %w", err),
		}, nil
	}

	log.Printf("[organizer] tv library selected: title=%q lib=%s reason=%s", tv.Title, selection.Library, selection.Reason)
	return o.organizeTVWithParsedAtLibrary(sourcePath, selection.Library, tv)
}

func (o *Organizer) OrganizeTVWithParsed(sourcePath, libraryPath string, tv naming.TVShowInfo) (*OrganizationResult, error) {
	return o.OrganizeTVWithParsedFinalized(sourcePath, libraryPath, tv, nil)
}

// OrganizeTVWithParsedFinalized holds the show-season lock through finalize.
func (o *Organizer) OrganizeTVWithParsedFinalized(sourcePath, libraryPath string, tv naming.TVShowInfo, finalize OrganizationFinalizer) (*OrganizationResult, error) {
	return organizeWithKeyLock(tvSeasonOrganizationLockKey(tv), finalize, func() (*OrganizationResult, error) {
		return o.organizeTVWithParsedAtLibrary(sourcePath, libraryPath, tv)
	})
}

func (o *Organizer) organizeTVWithParsedAtLibrary(sourcePath, libraryPath string, tv naming.TVShowInfo) (*OrganizationResult, error) {
	showDir := findExistingShowDir(libraryPath, tv.Title)
	reusingExistingShowDir := showDir != ""
	if showDir == "" {
		showDir = filepath.Join(libraryPath, naming.NormalizeMediaName(tv.Title, tv.Year))
	}
	seasonDir := findExistingSeasonDir(showDir, tv.Season)
	if seasonDir == "" {
		seasonDir = filepath.Join(showDir, naming.FormatSeasonFolder(tv.Season))
	}
	return o.organizeTVWithParsed(sourcePath, libraryPath, tv, showDir, seasonDir, reusingExistingShowDir)
}

// ponytail: One lock per show-season also serializes unrelated episodes in
// that season; range-aware locks can restore parallel episode imports later.
func tvSeasonOrganizationLockKey(tv naming.TVShowInfo) string {
	show := database.NormalizeForMatch(tv.Title)
	return fmt.Sprintf("tv-season:%s:%d", show, tv.Season)
}

func (o *Organizer) organizeTVWithParsed(sourcePath, libraryPath string, tv naming.TVShowInfo, showDir, seasonDir string, reusingExistingShowDir bool) (*OrganizationResult, error) {
	filename := filepath.Base(sourcePath)
	sourceQuality := quality.Parse(filename)

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

	var existingFile string
	var existingQuality *quality.QualityInfo
	var replacedPaths []string
	rangeEnd := tv.Episode
	if tv.EpisodeEnd > tv.Episode {
		rangeEnd = tv.EpisodeEnd
	}
	replacedPaths, err := findEpisodeFilesForRange(seasonDir, tv.Season, tv.Episode, rangeEnd)
	if err != nil {
		return &OrganizationResult{
			SourcePath:    sourcePath,
			TargetPath:    targetPath,
			SourceQuality: sourceQuality,
			Error:         fmt.Errorf("unable to inspect existing TV episode files: %w", err),
		}, nil
	}
	incomingRange := fmt.Sprintf("S%02dE%02d", tv.Season, tv.Episode)
	if rangeEnd > tv.Episode {
		incomingRange += fmt.Sprintf("-E%02d", rangeEnd)
	}
	for _, overlap := range replacedPaths {
		fileSeason, fileStart, fileEnd, ok := extractEpisodeRange(filepath.Base(overlap))
		if !ok {
			continue
		}
		overlapEpisode := fileStart
		if overlapEpisode < tv.Episode {
			overlapEpisode = tv.Episode
		}
		fileRange := fmt.Sprintf("S%02dE%02d", fileSeason, fileStart)
		if fileEnd > fileStart {
			fileRange += fmt.Sprintf("-E%02d", fileEnd)
		}
		skipOverlap := func(reason string, qualityInfo *quality.QualityInfo) *OrganizationResult {
			return &OrganizationResult{
				SourcePath:      sourcePath,
				TargetPath:      overlap,
				Skipped:         true,
				SkipReason:      fmt.Sprintf("multi-episode range overlaps existing episode E%02d: %s", overlapEpisode, reason),
				SourceQuality:   sourceQuality,
				ExistingQuality: qualityInfo,
			}
		}

		if tv.EpisodeEnd > tv.Episode {
			if overlap == targetPath && o.alreadyPublished(sourcePath, overlap) {
				return &OrganizationResult{
					SourcePath: sourcePath,
					TargetPath: overlap,
					Skipped:    true,
					SkipReason: "already imported from this source",
				}, nil
			}
			if fileStart < tv.Episode || fileEnd > rangeEnd {
				return skipOverlap(fmt.Sprintf("existing range %s extends outside incoming range %s", fileRange, incomingRange), nil), nil
			}
			if !hasQualityEvidence(sourceQuality) {
				return skipOverlap("incoming source has no comparable quality data", nil), nil
			}
			if o.db == nil {
				return skipOverlap("successful import quality evidence is unavailable", nil), nil
			}
			decision, err := o.db.GetDecisionByTargetPath(overlap)
			if err != nil {
				return skipOverlap(fmt.Sprintf("could not read successful import quality evidence: %v", err), nil), nil
			}
			if decision == nil || strings.TrimSpace(decision.SourceFilename) == "" {
				return skipOverlap("successful import quality evidence is unavailable", nil), nil
			}
			importedQuality := quality.Parse(decision.SourceFilename)
			if !hasQualityEvidence(importedQuality) {
				return skipOverlap("recorded source filename has no comparable quality data", importedQuality), nil
			}
			if !sourceQuality.IsBetterThan(importedQuality) {
				return skipOverlap(fmt.Sprintf("incoming quality (%s) is not higher than imported quality (%s)", sourceQuality.String(), importedQuality.String()), importedQuality), nil
			}
			if existingQuality == nil {
				existingQuality = importedQuality
			}
		} else if fileStart < tv.Episode || fileEnd > rangeEnd {
			return skipOverlap(fmt.Sprintf("existing range %s extends outside incoming range %s", fileRange, incomingRange), nil), nil
		}
	}
	if tv.EpisodeEnd > tv.Episode {
		existingFile = ""
	} else if len(replacedPaths) > 0 {
		existingFile = replacedPaths[0]
		if skipped := alreadyOrganizedResult(sourcePath, targetPath); skipped != nil {
			return skipped, nil
		}
		if o.alreadyPublished(sourcePath, existingFile) {
			return &OrganizationResult{
				SourcePath: sourcePath,
				TargetPath: existingFile,
				Skipped:    true,
				SkipReason: "already imported from this source",
			}, nil
		}
		if !o.forceOverwrite {
			existingQuality, err = o.qualityForExistingTarget(existingFile)
			if err != nil {
				return &OrganizationResult{
					SourcePath:    sourcePath,
					TargetPath:    existingFile,
					Skipped:       true,
					SkipReason:    fmt.Sprintf("could not read existing quality evidence: %v", err),
					SourceQuality: sourceQuality,
				}, nil
			}
			if reason := qualityReplacementSkipReason(sourceQuality, existingQuality); reason != "" {
				return &OrganizationResult{
					Success:         false,
					SourcePath:      sourcePath,
					TargetPath:      existingFile,
					Skipped:         true,
					SkipReason:      reason,
					SourceQuality:   sourceQuality,
					ExistingQuality: existingQuality,
				}, nil
			}
		}
		replacedPaths = []string{existingFile}
	} else {
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

	result, sourceRetained, err := o.transferSource(sourcePath, targetPath, opts, showDir, libraryPath, existingFile == targetPath)

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

	var removedPaths []string
	var replacementErrs []error
	replacedExisting := existingFile == targetPath
	for _, replacedPath := range replacedPaths {
		if replacedPath == targetPath || replacedPath == sourcePath {
			continue
		}
		if err := os.Remove(replacedPath); err != nil && !os.IsNotExist(err) {
			replacementErrs = append(replacementErrs, fmt.Errorf("remove replaced episode %s: %w", replacedPath, err))
			continue
		}
		removedPaths = append(removedPaths, replacedPath)
		replacedExisting = true
	}

	// See OrganizeMovieWithParsed: the move already completed, so an indexing
	// failure is reported without demoting the organize to failed.
	inventoryErr := errors.Join(errors.Join(replacementErrs...), o.refreshInventoryAfterTransfer(sourcePath, targetPath, removedPaths, libraryPath, showDir, "episode", sourceRetained))

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
				Success:           false,
				SourcePath:        sourcePath,
				TargetPath:        targetPath,
				BytesCopied:       result.BytesCopied,
				Duration:          result.Duration,
				Attempts:          result.Attempts,
				SourceQuality:     sourceQuality,
				sourceRetained:    sourceRetained,
				publishedIdentity: publishedMediaIdentity(result, targetPath),
				Error:             fmt.Errorf("DB upsert failed after file move — filesystem and database are inconsistent: %w", err),
			}, nil
		}
		if o.syncService != nil {
			o.db.SetSeriesDirty(seriesRecord.ID)
			o.syncService.QueueSync("series", seriesRecord.ID)
		}
	}

	return &OrganizationResult{
		Success:           true,
		SourcePath:        sourcePath,
		TargetPath:        targetPath,
		BytesCopied:       result.BytesCopied,
		Duration:          result.Duration,
		Attempts:          result.Attempts,
		SourceQuality:     sourceQuality,
		ExistingQuality:   existingQuality,
		InventoryError:    inventoryErr,
		sourceRetained:    sourceRetained,
		publishedIdentity: publishedMediaIdentity(result, targetPath),
		replacedExisting:  replacedExisting,
	}, nil
}

func (o *Organizer) OrganizeTVEpisodeAuto(sourcePath string, getFileSize func(string) (int64, error)) (*OrganizationResult, error) {
	return o.OrganizeTVEpisodeAutoFinalized(sourcePath, getFileSize, nil)
}

// OrganizeTVEpisodeAutoFinalized parses and selects under the show-season lock.
func (o *Organizer) OrganizeTVEpisodeAutoFinalized(sourcePath string, getFileSize func(string) (int64, error), finalize OrganizationFinalizer) (*OrganizationResult, error) {
	info, err := getFileSize(sourcePath)
	if err != nil {
		return finalizeOrganization(&OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to get file size: %w", err),
		}, nil, finalize)
	}

	tv, err := naming.ParseTVShowFromPath(sourcePath)
	if err != nil {
		return finalizeOrganization(&OrganizationResult{
			Success:    false,
			SourcePath: sourcePath,
			Error:      fmt.Errorf("unable to parse TV show name: %w", err),
		}, nil, finalize)
	}

	return o.organizeTVWithParsedAutoFinalized(sourcePath, *tv, info, finalize)
}

func (o *Organizer) OrganizeTVSeasonPackAuto(releaseDir string, getFileSize func(string) (int64, error)) (*SeasonPackResult, error) {
	return o.OrganizeTVSeasonPackAutoFinalized(releaseDir, getFileSize, nil)
}

// OrganizeTVSeasonPackAutoFinalized holds the affected show-season locks through
// pack rollback and finalization so another import cannot observe an unfinished pack.
func (o *Organizer) OrganizeTVSeasonPackAutoFinalized(releaseDir string, getFileSize func(string) (int64, error), finalize SeasonPackFinalizer) (*SeasonPackResult, error) {
	releaseDir = filepath.Clean(releaseDir)
	packInfo, err := naming.ParseTVSeasonPackName(filepath.Base(releaseDir))
	if err != nil {
		if finalize != nil {
			finalize(nil, err)
		}
		return nil, err
	}

	videoFiles, err := collectVideoFiles(releaseDir)
	if err != nil {
		if finalize != nil {
			finalize(nil, err)
		}
		return nil, err
	}

	result := &SeasonPackResult{SourceDir: releaseDir}
	type seasonPackEpisode struct {
		path string
		tv   naming.TVShowInfo
	}
	episodes := make([]seasonPackEpisode, 0, len(videoFiles))
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
		episodes = append(episodes, seasonPackEpisode{path: path, tv: *tv})
	}

	if len(episodes) == 0 {
		if naming.IsExtrasRelease(filepath.Base(releaseDir)) {
			result, err = o.organizeExtrasRelease(releaseDir, packInfo, result)
		} else {
			result.Skipped = true
			result.SkipReason = "season_pack_unresolved"
			result.Error = fmt.Errorf("season pack has no episode-numbered video files: %s", releaseDir)
		}
		if finalize != nil {
			finalize(result, err)
		}
		return result, err
	}

	keys := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		keys = append(keys, tvSeasonOrganizationLockKey(episode.tv))
	}
	var runErr error
	func() {
		unlock, err := lockOrganizationKeys(keys)
		if err != nil {
			runErr = fmt.Errorf("acquire season-pack organizer locks: %w", err)
			result.Error = runErr
			if finalize != nil {
				finalize(result, runErr)
			}
			return
		}
		defer unlock()

		for _, episode := range episodes {
			info, sizeErr := getFileSize(episode.path)
			var itemResult *OrganizationResult
			var itemErr error
			if sizeErr != nil {
				itemResult = &OrganizationResult{
					SourcePath: episode.path,
					Error:      fmt.Errorf("unable to get file size: %w", sizeErr),
				}
			} else {
				itemResult, itemErr = o.organizeTVWithParsedAuto(episode.path, episode.tv, info)
			}
			if itemErr != nil {
				result.Error = itemErr
				runErr = itemErr
				result.FailedEpisode = itemResult
				if result.FailedEpisode == nil {
					result.FailedEpisode = &OrganizationResult{SourcePath: episode.path, Error: itemErr}
				}
				if finalize != nil {
					finalize(result, runErr)
				}
				return
			}
			result.Imported = append(result.Imported, itemResult)
			if itemResult != nil {
				result.BytesCopied += itemResult.BytesCopied
				if itemResult.Error != nil && !itemResult.Skipped {
					result.Error = itemResult.Error
					result.FailedEpisode = itemResult
					result.SkipReason = "season_pack_partial_failure"
					if rollbackErr := o.rollbackSeasonPackImports(result.Imported); rollbackErr != nil {
						result.Error = fmt.Errorf("%w; rollback failed: %v", result.Error, rollbackErr)
					}
					result.Success = false
					if finalize != nil {
						finalize(result, nil)
					}
					return
				}
			}
		}

		result.Success = result.Error == nil
		if !result.Success && result.SkipReason == "" {
			result.SkipReason = "season_pack_partial_failure"
		}
		if finalize != nil {
			finalize(result, nil)
		}
	}()
	return result, runErr
}

// organizeExtrasRelease routes a bonus/extras release into the matching show's
// extras/ folder (Jellyfin's extras convention). It only matches existing show
// directories — an extras release for a show not present in any library is
// terminally skipped so it stops being retried.
func (o *Organizer) organizeExtrasRelease(releaseDir string, packInfo *naming.TVSeasonPackInfo, result *SeasonPackResult) (*SeasonPackResult, error) {
	showDir := ""
	libraryRoot := ""
	for _, lib := range o.libraries {
		if packInfo.Year != "" {
			if dir := findExistingShowDir(lib, naming.NormalizeMediaName(packInfo.Title, packInfo.Year)); dir != "" {
				showDir = dir
				libraryRoot = lib
				break
			}
		}
		if dir := findExistingShowDir(lib, packInfo.Title); dir != "" {
			if packInfo.Year != "" {
				year := 0
				_, _ = fmt.Sscanf(packInfo.Year, "%d", &year)
				decision := identity.CompareSeries(
					identity.SeriesIdentity{Title: packInfo.Title, Year: year, Path: releaseDir},
					identity.SeriesIdentity{Path: dir},
				)
				if decision.Verdict != identity.VerdictSame {
					continue
				}
			}
			// ponytail: a yearless release cannot disambiguate same-title
			// remakes; require release metadata before adding remote lookup.
			showDir = dir
			libraryRoot = lib
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
	targets := make([]string, len(videos))
	seenTargets := make(map[string]struct{}, len(videos))
	for i, path := range videos {
		ext := filepath.Ext(path)
		cleaned := naming.CleanExtrasName(strings.TrimSuffix(filepath.Base(path), ext))
		if cleaned == "" {
			cleaned = strings.TrimSuffix(filepath.Base(path), ext)
		}
		targets[i] = filepath.Join(extrasDir, cleaned+ext)
		if _, exists := seenTargets[targets[i]]; exists {
			result.Unresolved = videos
			result.Skipped = true
			result.SkipReason = "extras_unresolved"
			result.Error = fmt.Errorf("multiple extras resolve to the same target: %s", targets[i])
			return result, nil
		}
		seenTargets[targets[i]] = struct{}{}
		if !o.forceOverwrite {
			if _, err := os.Lstat(targets[i]); err == nil || !os.IsNotExist(err) {
				result.Unresolved = videos
				result.Skipped = true
				result.SkipReason = "extras_unresolved"
				result.Error = fmt.Errorf("extras target already exists: %s", targets[i])
				return result, nil
			}
		}
	}
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
		targetPath := targets[i]
		item := &OrganizationResult{SourcePath: path, TargetPath: targetPath}
		if o.dryRun {
			item.Success = true
			result.Imported = append(result.Imported, item)
			continue
		}
		transferResult, sourceRetained, err := o.transferSource(path, targetPath, opts, showDir, libraryRoot, false)
		if err != nil {
			result.Unresolved = append(result.Unresolved, videos[i:]...)
			result.Error = fmt.Errorf("extras transfer failed: %w", err)
			result.Success = false
			return result, nil
		}
		item.Success = true
		item.sourceRetained = sourceRetained
		item.publishedIdentity = publishedMediaIdentity(transferResult, targetPath)
		if transferResult != nil {
			item.BytesCopied = transferResult.BytesCopied
			result.BytesCopied += transferResult.BytesCopied
		}
		result.Imported = append(result.Imported, item)
	}

	result.Success = true
	return result, nil
}

func (o *Organizer) rollbackSeasonPackImports(imported []*OrganizationResult) error {
	var errs []error
	for i := len(imported) - 1; i >= 0; i-- {
		item := imported[i]
		if item == nil || !item.Success || item.SourcePath == "" || item.TargetPath == "" {
			continue
		}
		if item.replacedExisting {
			// The old canonical file is already gone; keep this completed replacement
			// so a later episode failure cannot leave the library without either copy.
			continue
		}
		if _, err := os.Lstat(item.TargetPath); err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("stat target %s: %w", item.TargetPath, err))
			}
			continue
		}
		if item.publishedIdentity != "" && !fileidentity.MatchesLstat(item.TargetPath, item.publishedIdentity) {
			errs = append(errs, fmt.Errorf("published target changed before rollback: %s", item.TargetPath))
			continue
		}
		if err := o.waitIfScanning(o.timeout); err != nil {
			errs = append(errs, fmt.Errorf("wait before rollback %s -> %s: %w", item.TargetPath, item.SourcePath, err))
			continue
		}
		if item.sourceRetained {
			if _, err := os.Lstat(item.SourcePath); err != nil {
				errs = append(errs, fmt.Errorf("retained source unavailable, preserving target %s: %w", item.TargetPath, err))
				continue
			}
			if o.db != nil {
				if err := o.db.DeleteMediaFile(item.TargetPath); err != nil {
					errs = append(errs, fmt.Errorf("remove retained import row %s: %w", item.TargetPath, err))
					continue
				}
			}
			var err error
			if item.publishedIdentity != "" {
				err = fileidentity.RemoveIfMatches(item.TargetPath, item.publishedIdentity, true)
			} else {
				err = os.Remove(item.TargetPath)
			}
			if err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("remove retained import target %s: %w", item.TargetPath, err))
			}
			if o.db != nil {
				if err := o.db.RestoreBestFileForMediaFile(item.SourcePath); err != nil {
					errs = append(errs, fmt.Errorf("restore retained source best-file link %s: %w", item.SourcePath, err))
				}
			}
			continue
		}
		if _, err := os.Lstat(item.SourcePath); err == nil {
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
		libraryRoot := ""
		for _, root := range o.libraries {
			if config.PathContains(root, item.SourcePath) && len(root) > len(libraryRoot) {
				libraryRoot = root
			}
		}
		result, err := o.moveTrackedSource(item.TargetPath, item.SourcePath, o.buildTransferOptions(), "", libraryRoot, true, item.publishedIdentity, false)
		if err == nil && result != nil {
			err = result.Validate()
			if err == nil && !result.SourceRemoved {
				err = fmt.Errorf("rollback move did not remove target %s", item.TargetPath)
			}
		}
		if err != nil {
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
		if video.IsVideo(path) && !video.IsSampleVideo(path) {
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
		result, err := o.transferer.Copy(sub.Path, targetPath, opts)
		if err == nil {
			err = result.Validate()
		}
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
