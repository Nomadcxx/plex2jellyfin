package transfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
)

// DestinationMatches reports whether dst already holds src's bytes.
// The same file identity matches without reading the contents. Otherwise the
// size and checksum must both match.
func DestinationMatches(src, dst string) (bool, error) {
	if filepath.Clean(src) == filepath.Clean(dst) {
		return true, nil
	}
	dstInfo, err := StatWithTimeout(dst, 10*time.Second)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect destination: %w", err)
	}
	if !dstInfo.Mode().IsRegular() {
		return false, nil
	}
	srcInfo, err := StatWithTimeout(src, 10*time.Second)
	if err != nil {
		return false, fmt.Errorf("inspect source: %w", err)
	}
	if !srcInfo.Mode().IsRegular() {
		return false, nil
	}
	if os.SameFile(srcInfo, dstInfo) {
		return true, nil
	}
	if srcInfo.Size() != dstInfo.Size() {
		return false, nil
	}
	if _, err := verifyChecksum(src, dst); err != nil {
		if errors.Is(err, ErrChecksumMismatch) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// publishConflict reports that publishing failed because the destination
// already exists. Another attempt copies the file again and fails the same way.
func publishConflict(err error) bool {
	return errors.Is(err, os.ErrExist)
}

func finishTransferAttempts(lastErr error) error {
	if publishConflict(lastErr) {
		return lastErr
	}
	if lastErr == nil {
		return ErrRetryExhausted
	}
	return fmt.Errorf("%w: %w", ErrRetryExhausted, lastErr)
}

// takeExistingDestination completes a transfer that would only rewrite a file
// the destination already has. Database moves publish through OnPublished and
// keep the normal path so a pre-existing library file is not treated as a copy
// this transfer created.
func takeExistingDestination(src, dst string, opts TransferOptions, removeSource bool, started time.Time) (*TransferResult, error, bool) {
	if opts.OnPublished != nil {
		return nil, nil, false
	}
	matches, err := DestinationMatches(src, dst)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, false
		}
		return &TransferResult{Error: err}, err, true
	}
	if !matches {
		return nil, nil, false
	}
	info, err := os.Stat(dst)
	if err != nil {
		return &TransferResult{Error: fmt.Errorf("inspect identical destination: %w", err)}, err, true
	}
	if removeSource {
		result, err := completeIdenticalMove(src, dst, info.Size(), opts, started)
		return result, err, true
	}
	result := &TransferResult{
		Success:     true,
		Attempts:    1,
		BytesTotal:  info.Size(),
		BytesCopied: info.Size(),
		Duration:    time.Since(started),
	}
	if id, ok := fileidentity.FromLstat(dst); ok {
		result.PublishedIdentity = id
	}
	if opts.Checksum {
		sum, sumErr := checksumFile(src)
		if sumErr != nil {
			result.Success = false
			result.Error = sumErr
			return result, result.Error, true
		}
		result.Checksum = sum
	}
	return result, nil, true
}

func completeIdenticalMove(src, dst string, size int64, opts TransferOptions, started time.Time) (*TransferResult, error) {
	result := &TransferResult{
		Success:     true,
		Attempts:    1,
		BytesTotal:  size,
		BytesCopied: size,
		Duration:    time.Since(started),
	}
	if id, ok := fileidentity.FromLstat(dst); ok {
		result.PublishedIdentity = id
	}
	if opts.Checksum {
		sum, err := checksumFile(src)
		if err != nil {
			result.Success = false
			result.Error = err
			return result, result.Error
		}
		result.Checksum = sum
	}
	if filepath.Clean(src) == filepath.Clean(dst) {
		return result, nil
	}
	// Unlink only the source name. A hard link shares the destination inode,
	// and a second regular file must stay where it already is.
	if err := os.Remove(src); err != nil {
		result.Success = false
		result.Error = fmt.Errorf("remove identical source: %w", err)
		return result, result.Error
	}
	result.SourceRemoved = true
	return result, nil
}
