package transfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// HardlinkCopyTransferer publishes retained-source copies as hardlinks when
// possible and delegates cross-filesystem copies to the configured backend.
type HardlinkCopyTransferer struct {
	fallback Transferer
	link     func(string, string) error
}

func NewHardlinkCopyTransferer(fallback Transferer) *HardlinkCopyTransferer {
	return &HardlinkCopyTransferer{fallback: fallback, link: os.Link}
}

func (t *HardlinkCopyTransferer) Name() string {
	return "hardlink-first(" + t.fallback.Name() + ")"
}

func (t *HardlinkCopyTransferer) CanResume() bool {
	return t.fallback.CanResume()
}

func (t *HardlinkCopyTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	// Seed imports call Copy; Move retains the configured transferer's semantics.
	return t.fallback.Move(src, dst, opts)
}

func (t *HardlinkCopyTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	result := &TransferResult{Attempts: 1}
	started := time.Now()
	srcInfo, err := LstatWithTimeout(src, 10*time.Second)
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrSourceNotFound, err)
		return result, result.Error
	}
	copySource := src
	copyFallback := false
	if !srcInfo.Mode().IsRegular() {
		if srcInfo.Mode()&os.ModeSymlink == 0 {
			result.Error = fmt.Errorf("%w: %s", ErrSourceNotRegular, src)
			return result, result.Error
		}
		resolvedSource, err := EvalSymlinksWithTimeout(src, 10*time.Second)
		if err != nil {
			result.Error = fmt.Errorf("%w: resolve source symlink %s: %v", ErrSourceNotFound, src, err)
			return result, result.Error
		}
		resolvedInfo, err := LstatWithTimeout(resolvedSource, 10*time.Second)
		if err != nil {
			result.Error = fmt.Errorf("%w: inspect resolved source %s: %v", ErrSourceNotFound, resolvedSource, err)
			return result, result.Error
		}
		if !resolvedInfo.Mode().IsRegular() {
			result.Error = fmt.Errorf("%w: %s", ErrSourceNotRegular, src)
			return result, result.Error
		}
		copySource = resolvedSource
		srcInfo = resolvedInfo
		copyFallback = true
	}
	result.BytesTotal = srcInfo.Size()

	dirMode := opts.DirMode
	if dirMode == 0 {
		dirMode = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrDestinationNotWritable, err)
		return result, result.Error
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".link-*")
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrDestinationNotWritable, err)
		return result, result.Error
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		result.Error = fmt.Errorf("prepare hardlink destination: %w", err)
		return result, result.Error
	}
	if err := os.Remove(tmpPath); err != nil {
		result.Error = fmt.Errorf("prepare hardlink destination: %w", err)
		return result, result.Error
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if !copyFallback {
		if err := t.link(copySource, tmpPath); err != nil {
			if !hardlinkFallbackError(err) {
				result.Error = fmt.Errorf("create hardlink: %w", err)
				return result, result.Error
			}
			copyFallback = true
		}
	}
	if copyFallback {
		fallbackResult, err := t.fallback.Copy(copySource, tmpPath, opts)
		if err != nil {
			return fallbackResult, err
		}
		if fallbackResult == nil {
			result.Error = errors.New("copy fallback returned no result")
			return result, result.Error
		}
		if !fallbackResult.Success {
			return fallbackResult, nil
		}
		result = fallbackResult
	}
	linkedInfo, err := LstatWithTimeout(tmpPath, 10*time.Second)
	if err != nil {
		result.Error = fmt.Errorf("inspect staged transfer: %w", err)
		return result, result.Error
	}
	if !linkedInfo.Mode().IsRegular() {
		result.Error = fmt.Errorf("%w: staged transfer from %s", ErrSourceNotRegular, src)
		return result, result.Error
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		result.Error = fmt.Errorf("publish transfer: %w", err)
		return result, result.Error
	}
	committed = true
	result.Success = true
	result.Duration = time.Since(started)
	return result, nil
}

func hardlinkFallbackError(err error) bool {
	return errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.ENOSYS)
}
