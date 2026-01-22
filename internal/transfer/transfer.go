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
	"time"
)

// Common errors returned by transfer operations
var (
	// ErrTimeout is returned when a transfer times out due to no progress
	ErrTimeout = errors.New("transfer timed out: no progress")

	// ErrChecksumMismatch is returned when post-transfer checksum verification fails
	ErrChecksumMismatch = errors.New("checksum mismatch after transfer")

	// ErrSourceNotFound is returned when the source file doesn't exist
	ErrSourceNotFound = errors.New("source file not found")

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

	// Attempts is the number of attempts made (including retries)
	Attempts int

	// Error contains the error if Success is false
	Error error
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
		if pvPath, err := exec.LookPath("pv"); err == nil {
			return NewPVTransferer(pvPath), nil
		}
		if rsyncPath, err := exec.LookPath("rsync"); err == nil {
			return NewRsyncTransferer(rsyncPath), nil
		}
		return NewNativeTransferer(32 * 1024 * 1024), nil
	}
}

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
