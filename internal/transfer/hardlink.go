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
	return t.fallback.Move(src, dst, opts)
}

func (t *HardlinkCopyTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	result := &TransferResult{Attempts: 1}
	started := time.Now()
	srcInfo, err := StatWithTimeout(src, 10*time.Second)
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrSourceNotFound, err)
		return result, result.Error
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

	if err := t.link(src, tmpPath); err != nil {
		if hardlinkFallbackError(err) {
			return t.fallback.Copy(src, dst, opts)
		}
		result.Error = fmt.Errorf("create hardlink: %w", err)
		return result, result.Error
	}
	if !opts.PreserveTimes {
		now := time.Now()
		if err := os.Chtimes(tmpPath, now, now); err != nil {
			result.Error = fmt.Errorf("set import time: %w", err)
			return result, result.Error
		}
	}
	if err := ApplyPermissions(tmpPath, opts); err != nil {
		result.Error = fmt.Errorf("apply transfer permissions: %w", err)
		return result, result.Error
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		result.Error = fmt.Errorf("publish hardlink: %w", err)
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
