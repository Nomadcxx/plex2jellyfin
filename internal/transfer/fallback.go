package transfer

import (
	"fmt"
	"strings"
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
	// Can resume if any backend can resume
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
	var lastResult *TransferResult
	var lastErr error
	var errors []string

	for i, backend := range f.backends {
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

		// If not last backend, log and try next
		if i < len(f.backends)-1 {
			fmt.Printf("  ⚠️  %s failed, trying %s...\n", backend.Name(), f.backends[i+1].Name())
		}
	}

	// All backends failed
	if lastResult != nil {
		lastResult.Error = fmt.Errorf("all backends failed: %s", strings.Join(errors, "; "))
		return lastResult, lastResult.Error
	}

	return &TransferResult{
		Success: false,
		Error:   fmt.Errorf("all backends failed: %s", strings.Join(errors, "; ")),
	}, lastErr
}
