// Package transfer provides robust file transfer implementations with timeout,
// retry, progress tracking, and checksum verification. It solves the problem
// of os.Rename() and io.Copy() hanging indefinitely on failing disks.
//
// The primary implementation uses rsync as a backend, which has built-in
// timeout handling (--timeout flag) that aborts transfers when no progress
// is made for a specified duration.
package transfer

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
)

// Common errors returned by transfer operations
var (
	// ErrTimeout is returned when a transfer times out due to no progress
	ErrTimeout = errors.New("transfer timed out: no progress")

	// ErrChecksumMismatch is returned when post-transfer checksum verification fails
	ErrChecksumMismatch = errors.New("checksum mismatch after transfer")

	// ErrSourceNotFound is returned when the source file doesn't exist
	ErrSourceNotFound = errors.New("source file not found")

	// ErrSourceNotRegular is returned when a source resolves to a non-regular file.
	ErrSourceNotRegular = errors.New("source is not a regular file")

	// ErrDestinationNotWritable is returned when the destination is not writable
	ErrDestinationNotWritable = errors.New("destination not writable")

	// ErrDiskUnhealthy is returned when pre-flight disk health check fails
	ErrDiskUnhealthy = errors.New("disk health check failed")

	// ErrTransferFailed is returned when a transfer fails for unspecified reasons
	ErrTransferFailed = errors.New("transfer failed")

	// ErrRetryExhausted is returned when all retry attempts have been exhausted
	ErrRetryExhausted = errors.New("all retry attempts exhausted")
)

// TransferOptions configures the behavior of a file transfer operation.
type TransferOptions struct {
	// Timeout specifies how long to wait without progress before aborting.
	// A value of 0 means no timeout.
	Timeout time.Duration

	// Checksum enables post-transfer checksum verification.
	Checksum bool

	// Progress is called periodically with transfer progress updates.
	// current is bytes transferred so far, total is total bytes to transfer.
	// total may be -1 if unknown.
	Progress func(current, total int64)

	// RetryAttempts specifies how many times to retry on transient failures.
	// A value of 0 means no retries.
	RetryAttempts int

	// RetryDelay specifies how long to wait between retry attempts.
	RetryDelay time.Duration

	// PreserveAttrs preserves file ownership and permissions during transfer.
	PreserveAttrs bool

	// PreserveTimes preserves source modification times during transfer.
	// Hardlink copies share the source inode: true preserves its existing mtime
	// at both paths, while false sets the import time at both paths. Default
	// false makes Jellyfin's Recently Added reflect when the file entered the
	// library; true can preserve an old Recently Added time.
	PreserveTimes bool

	// DeletePartial removes partial files if transfer fails.
	// If false, partial files are left for potential resumption.
	DeletePartial bool

	// TargetUID sets the owner of transferred files. A value of -1 means
	// preserve source ownership (or process owner if PreserveAttrs is false).
	TargetUID int

	// TargetGID sets the group of transferred files. A value of -1 means
	// preserve source group (or process group if PreserveAttrs is false).
	TargetGID int

	// FileMode sets the permissions for transferred files. A value of 0 means
	// preserve source permissions (or use umask default if PreserveAttrs is false).
	FileMode os.FileMode

	// DirMode sets the permissions for created directories. A value of 0 means
	// use 0755 default.
	DirMode os.FileMode

	// SkipHealthCheck bypasses the per-backend pre-flight CheckDiskHealthForTransfer.
	// Set by orchestrators (e.g. FallbackTransferer) that have already verified
	// disk health to avoid redundant write probes that contend with active streams.
	SkipHealthCheck bool

	// OnPublished persists destination identity and recovery state before Move
	// removes the source.
	OnPublished func(identity, destinationQuarantinePath string) error

	// MoveSourceIdentity and MoveSourceQuarantinePath are set by the database
	// move reservation so source removal can be recovered after a crash.
	MoveSourceIdentity       string
	MoveSourceQuarantinePath string
}

// DefaultOptions returns sensible default transfer options.
func DefaultOptions() TransferOptions {
	return TransferOptions{
		Timeout:       5 * time.Minute,
		Checksum:      false,
		Progress:      nil,
		RetryAttempts: 3,
		RetryDelay:    5 * time.Second,
		PreserveAttrs: true,
		DeletePartial: false,
		TargetUID:     -1,
		TargetGID:     -1,
		FileMode:      0,
		DirMode:       0,
	}
}

// TransferResult contains details about a completed transfer operation.
type TransferResult struct {
	// Success indicates whether the transfer completed successfully
	Success bool

	// BytesTotal is the total size of the source file
	BytesTotal int64

	// BytesCopied is the number of bytes actually transferred
	BytesCopied int64

	// Duration is how long the transfer took
	Duration time.Duration

	// Checksum is the checksum of the transferred file (if verification was enabled)
	Checksum string

	// SourceRemoved indicates whether the source file was deleted (for Move operations)
	SourceRemoved bool

	// Published indicates that the destination path was created by this transfer.
	Published bool

	// PublishedIdentity identifies the destination inode and its basic file version.
	PublishedIdentity string

	// DestinationQuarantinePath stores the published destination while the
	// source quarantine is removed.
	DestinationQuarantinePath string

	// PendingSourceRemoval signals that Move timed out while unlinking the source.
	// The channel resolves when that unlink finishes and any failed move is cleaned up.
	PendingSourceRemoval <-chan error

	// Attempts is the number of attempts made (including retries)
	Attempts int

	// Error contains the error if Success is false
	Error error
}

// Validate rejects transfer results that do not confirm a completed operation.
func (r *TransferResult) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: backend returned no result", ErrTransferFailed)
	}
	if r.Success {
		return nil
	}
	if r.Error != nil {
		return r.Error
	}
	return fmt.Errorf("%w: backend reported unsuccessful result without an error", ErrTransferFailed)
}

// Transferer is the interface for file transfer implementations.
// Implementations must be safe for concurrent use.
type Transferer interface {
	// Move transfers a file from src to dst, then removes the source.
	// The source is only removed after successful transfer verification.
	// Returns ErrSourceNotFound if source doesn't exist.
	// Returns ErrDestinationNotWritable if destination isn't writable.
	// Returns ErrTimeout if no progress is made within opts.Timeout.
	Move(src, dst string, opts TransferOptions) (*TransferResult, error)

	// Copy transfers a file from src to dst without removing the source.
	// Same error conditions as Move.
	Copy(src, dst string, opts TransferOptions) (*TransferResult, error)

	// CanResume returns true if this transferer supports resuming
	// interrupted transfers.
	CanResume() bool

	// Name returns a human-readable name for this transferer implementation.
	Name() string
}

// RollbackMove returns a completed move to its original source path. It only
// restores when that path is still absent, so it cannot overwrite new data.
func RollbackMove(t Transferer, sourcePath, destinationPath string, opts TransferOptions, expectedIdentity string) error {
	if _, err := os.Lstat(sourcePath); err == nil {
		return fmt.Errorf("cannot restore moved file because source path now exists: %s", sourcePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect original source path %s: %w", sourcePath, err)
	}
	if expectedIdentity == "" {
		return fmt.Errorf("cannot restore moved file without its published destination identity: %s", destinationPath)
	}
	if _, err := os.Lstat(destinationPath); err != nil {
		return fmt.Errorf("inspect moved destination %s: %w", destinationPath, err)
	}
	if !fileidentity.MatchesLstat(destinationPath, expectedIdentity) {
		return fmt.Errorf("cannot restore moved file because destination changed: %s", destinationPath)
	}

	opts.OnPublished = nil
	quarantinePath, err := fileidentity.NewQuarantinePath(destinationPath)
	if err != nil {
		return err
	}
	opts.MoveSourceIdentity = expectedIdentity
	opts.MoveSourceQuarantinePath = quarantinePath
	result, err := t.Move(destinationPath, sourcePath, opts)
	if err != nil {
		return fmt.Errorf("move file back to %s: %w", sourcePath, err)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("move file back to %s: %w", sourcePath, err)
	}
	if !result.SourceRemoved {
		return fmt.Errorf("move back to %s did not remove destination %s", sourcePath, destinationPath)
	}
	return nil
}

func cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, publishedIdentity string, moveErr error) error {
	if _, err := os.Lstat(sourcePath); errors.Is(err, os.ErrNotExist) {
		return moveErr
	} else if err != nil {
		return errors.Join(moveErr, fmt.Errorf("inspect source after failed move: %w", err))
	}
	if errors.Is(moveErr, ErrTimeout) {
		return fmt.Errorf("%w; destination retained because source removal may still complete", moveErr)
	}

	if publishedIdentity == "" {
		return fmt.Errorf("%w; published destination identity is unavailable, leaving it in place", moveErr)
	}
	if err := fileidentity.RemoveIfMatches(destinationPath, publishedIdentity, true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return moveErr
		}
		return errors.Join(moveErr, fmt.Errorf("remove published destination after failed move: %w", err))
	}
	return moveErr
}

func samePublishedFile(publishedInfo, currentInfo os.FileInfo) bool {
	return publishedInfo != nil && currentInfo != nil && os.SameFile(publishedInfo, currentInfo) &&
		publishedInfo.Size() == currentInfo.Size() && publishedInfo.ModTime().Equal(currentInfo.ModTime())
}

func recordPublishedDestination(result *TransferResult, sourcePath, destinationPath string, opts TransferOptions) (os.FileInfo, error) {
	result.Published = true
	publishedInfo, err := LstatWithTimeout(destinationPath, 10*time.Second)
	if err != nil {
		result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, "", fmt.Errorf("inspect published destination: %w", err))
		return nil, result.Error
	}
	if identity, ok := fileidentity.FromLstat(destinationPath); ok {
		currentInfo, statErr := LstatWithTimeout(destinationPath, 10*time.Second)
		if statErr != nil || !samePublishedFile(publishedInfo, currentInfo) {
			if statErr == nil {
				statErr = fmt.Errorf("destination changed while recording its identity")
			}
			result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, identity, fmt.Errorf("inspect published identity: %w", statErr))
			return publishedInfo, result.Error
		}
		result.PublishedIdentity = identity
	} else {
		result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, "", errors.New("platform does not expose a stable destination identity"))
		return publishedInfo, result.Error
	}
	publishedContentInfo, err := StatWithTimeout(destinationPath, 10*time.Second)
	if err != nil {
		result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, result.PublishedIdentity, fmt.Errorf("published destination is not readable: %w", err))
		return publishedInfo, result.Error
	}
	if !publishedContentInfo.Mode().IsRegular() {
		result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, result.PublishedIdentity, fmt.Errorf("published destination is not a regular file: %s", destinationPath))
		return publishedInfo, result.Error
	}
	destinationQuarantinePath, err := fileidentity.NewQuarantinePath(destinationPath)
	if err != nil {
		result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, result.PublishedIdentity, fmt.Errorf("reserve destination recovery path: %w", err))
		return publishedInfo, result.Error
	}
	result.DestinationQuarantinePath = destinationQuarantinePath
	if opts.OnPublished != nil {
		if err := opts.OnPublished(result.PublishedIdentity, destinationQuarantinePath); err != nil {
			result.Error = cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, result.PublishedIdentity, fmt.Errorf("record published destination: %w", err))
			return publishedInfo, result.Error
		}
	}
	return publishedInfo, nil
}

func resolvePendingSourceRemoval(sourcePath, destinationPath string, publishedInfo os.FileInfo, publishedIdentity string, pending <-chan error) <-chan error {
	resolved := make(chan error, 1)
	go func() {
		removeErr := <-pending
		if removeErr == nil {
			currentInfo, err := LstatWithTimeout(destinationPath, 10*time.Second)
			if err != nil {
				resolved <- fmt.Errorf("inspect published destination after source removal: %w", err)
				return
			}
			if !samePublishedFile(publishedInfo, currentInfo) {
				resolved <- fmt.Errorf("published destination changed while source removal was pending: %s", destinationPath)
				return
			}
			resolved <- nil
			return
		}

		moveErr := fmt.Errorf("remove source after publish: %w", removeErr)
		cleanupErr := cleanupPublishedDestinationAfterMoveFailure(sourcePath, destinationPath, publishedIdentity, moveErr)
		_, sourceErr := os.Lstat(sourcePath)
		destinationInfo, destinationErr := os.Lstat(destinationPath)
		if errors.Is(sourceErr, os.ErrNotExist) && destinationErr == nil && samePublishedFile(publishedInfo, destinationInfo) {
			resolved <- nil
			return
		}
		if sourceErr == nil && errors.Is(destinationErr, os.ErrNotExist) {
			resolved <- nil
			return
		}
		resolved <- errors.Join(cleanupErr, fmt.Errorf("source removal left an unresolved move from %s to %s", sourcePath, destinationPath))
	}()
	return resolved
}

func prepareSourceRemoval(src string, opts TransferOptions) (TransferOptions, error) {
	if opts.MoveSourceIdentity == "" {
		identity, ok := fileidentity.FromLstat(src)
		if !ok {
			return opts, fmt.Errorf("source has no stable file identity: %s", src)
		}
		opts.MoveSourceIdentity = identity
	}
	if opts.MoveSourceQuarantinePath == "" {
		quarantine, err := fileidentity.NewQuarantinePath(src)
		if err != nil {
			return opts, err
		}
		opts.MoveSourceQuarantinePath = quarantine
	}
	if filepath.Clean(opts.MoveSourceQuarantinePath) == filepath.Clean(src) ||
		filepath.Clean(filepath.Dir(opts.MoveSourceQuarantinePath)) != filepath.Clean(filepath.Dir(src)) {
		return opts, fmt.Errorf("source quarantine must be a distinct sibling of %s", src)
	}
	return opts, nil
}

// RenameMove atomically moves a tracked source on one filesystem without
// overwriting a destination or accidentally moving a replacement at src.
func RenameMove(src, dst string, opts TransferOptions) (*TransferResult, error) {
	opts, err := prepareSourceRemoval(src, opts)
	if err != nil {
		return nil, err
	}
	targetQuarantinePath, err := fileidentity.NewQuarantinePath(dst)
	if err != nil {
		return nil, err
	}
	if err := fileidentity.MoveToQuarantine(src, opts.MoveSourceQuarantinePath, opts.MoveSourceIdentity, true); err != nil {
		return nil, err
	}
	restoreSource := func(cause error) (*TransferResult, error) {
		return nil, errors.Join(cause, fileidentity.RestoreQuarantine(opts.MoveSourceQuarantinePath, src, opts.MoveSourceIdentity, true))
	}
	if opts.OnPublished != nil {
		if err := opts.OnPublished(opts.MoveSourceIdentity, targetQuarantinePath); err != nil {
			return restoreSource(fmt.Errorf("record atomic move destination: %w", err))
		}
	}
	if err := fileidentity.RenameNoReplace(opts.MoveSourceQuarantinePath, dst); err != nil {
		return restoreSource(fmt.Errorf("publish atomic move destination: %w", err))
	}
	return &TransferResult{
		Success:                   true,
		SourceRemoved:             true,
		Published:                 true,
		PublishedIdentity:         opts.MoveSourceIdentity,
		DestinationQuarantinePath: targetQuarantinePath,
	}, nil
}

func removeSourceWithTimeout(src, sourceQuarantinePath, sourceIdentity, dst, destinationQuarantinePath, destinationIdentity string, timeout time.Duration) (<-chan error, error) {
	completed := make(chan error, 1)
	go func() {
		err := fileidentity.MoveToQuarantine(src, sourceQuarantinePath, sourceIdentity, true)
		if err == nil {
			err = fileidentity.MoveToQuarantine(dst, destinationQuarantinePath, destinationIdentity, true)
			if err != nil {
				err = errors.Join(err, fileidentity.RestoreQuarantine(sourceQuarantinePath, src, sourceIdentity, true))
				completed <- err
				return
			}
			err = fileidentity.RemoveQuarantine(sourceQuarantinePath, sourceIdentity, true)
			if err != nil {
				err = errors.Join(err,
					fileidentity.RestoreQuarantine(sourceQuarantinePath, src, sourceIdentity, true),
					fileidentity.RestoreQuarantine(destinationQuarantinePath, dst, destinationIdentity, true))
				completed <- err
				return
			}
			err = fileidentity.RestoreQuarantine(destinationQuarantinePath, dst, destinationIdentity, true)
			if err != nil {
				// The published file remains in its persisted quarantine for startup recovery.
				err = fmt.Errorf("restore published destination %s: %w", dst, err)
			}
		}
		completed <- err
	}()

	select {
	case err := <-completed:
		return nil, err
	case <-time.After(timeout):
		return completed, fmt.Errorf("%w: source quarantine timed out after %s for path: %s", ErrTimeout, timeout, src)
	}
}

type Backend int

const (
	BackendAuto Backend = iota
	BackendPV
	BackendRsync
	BackendNative
)

func (b Backend) String() string {
	switch b {
	case BackendAuto:
		return "auto"
	case BackendPV:
		return "pv"
	case BackendRsync:
		return "rsync"
	case BackendNative:
		return "native"
	default:
		return "unknown"
	}
}

func New(backend Backend) (Transferer, error) {
	switch backend {
	case BackendPV:
		pvPath, err := exec.LookPath("pv")
		if err != nil {
			return nil, fmt.Errorf("pv not found: %w", err)
		}
		return NewPVTransferer(pvPath), nil

	case BackendRsync:
		rsyncPath, err := exec.LookPath("rsync")
		if err != nil {
			return nil, fmt.Errorf("rsync not found: %w", err)
		}
		return NewRsyncTransferer(rsyncPath), nil

	case BackendNative:
		return NewNativeTransferer(32 * 1024 * 1024), nil

	case BackendAuto:
		fallthrough
	default:
		// Build fallback chain: rsync -> pv -> native
		var backends []Transferer

		if rsyncPath, err := exec.LookPath("rsync"); err == nil {
			backends = append(backends, NewRsyncTransferer(rsyncPath))
		}
		if pvPath, err := exec.LookPath("pv"); err == nil {
			backends = append(backends, NewPVTransferer(pvPath))
		}
		// Native is always available as final fallback
		backends = append(backends, NewNativeTransferer(32*1024*1024))

		if len(backends) == 1 {
			// Only native available, return it directly
			return backends[0], nil
		}
		return NewFallbackTransferer(backends...), nil
	}
}

// MustNew creates a new Transferer with the specified backend.
// Deprecated: Use New instead. MustNew panics on error which is not recommended
// for production code. Migrate to New(backend) (Transferer, error) for proper
// error handling.
func MustNew(backend Backend) Transferer {
	t, err := New(backend)
	if err != nil {
		panic(fmt.Sprintf("transfer.MustNew: %v", err))
	}
	return t
}

func ParseBackend(s string) Backend {
	switch s {
	case "pv":
		return BackendPV
	case "rsync":
		return BackendRsync
	case "native":
		return BackendNative
	default:
		return BackendAuto
	}
}
