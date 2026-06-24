package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FallbackTransferer tries multiple backends in order until one succeeds.
type FallbackTransferer struct {
	backends []Transferer
}

// NewFallbackTransferer creates a transferer that tries backends in order.
// If the first backend fails, it tries the next, and so on.
func NewFallbackTransferer(backends ...Transferer) *FallbackTransferer {
	return &FallbackTransferer{backends: backends}
}

func (f *FallbackTransferer) Name() string {
	names := make([]string, len(f.backends))
	for i, b := range f.backends {
		names[i] = b.Name()
	}
	return "fallback(" + strings.Join(names, ",") + ")"
}

func (f *FallbackTransferer) CanResume() bool {
	for _, b := range f.backends {
		if b.CanResume() {
			return true
		}
	}
	return false
}

func (f *FallbackTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return f.tryAll(src, dst, opts, true)
}

func (f *FallbackTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return f.tryAll(src, dst, opts, false)
}

func (f *FallbackTransferer) tryAll(src, dst string, opts TransferOptions, isMove bool) (*TransferResult, error) {
	// Single upfront health probe shared by all backends. Avoids 3x redundant
	// write probes contending with active concurrent transfers on the same disk.
	if !opts.SkipHealthCheck {
		var requiredSpace int64
		if info, err := os.Stat(src); err == nil {
			requiredSpace = info.Size()
		}
		if err := CheckDiskHealthForTransfer(src, dst, 30*time.Second, requiredSpace); err != nil {
			_ = filepath.Dir(dst)
			return &TransferResult{
				Success: false,
				Error:   fmt.Errorf("%w: %v", ErrDiskUnhealthy, err),
			}, fmt.Errorf("%w: %v", ErrDiskUnhealthy, err)
		}
		opts.SkipHealthCheck = true
	}

	var lastResult *TransferResult
	var lastErr error
	var errors []string

	for _, backend := range f.backends {
		var result *TransferResult
		var err error

		if isMove {
			result, err = backend.Move(src, dst, opts)
		} else {
			result, err = backend.Copy(src, dst, opts)
		}

		if err == nil && result.Success {
			return result, nil
		}

		lastResult = result
		lastErr = err
		errMsg := fmt.Sprintf("%s: %v", backend.Name(), err)
		errors = append(errors, errMsg)
	}

	if lastResult != nil {
		lastResult.Error = fmt.Errorf("all backends failed: %s", strings.Join(errors, "; "))
		return lastResult, lastResult.Error
	}

	return &TransferResult{
		Success: false,
		Error:   fmt.Errorf("all backends failed: %s", strings.Join(errors, "; ")),
	}, lastErr
}
