package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

type NativeTransferer struct {
	bufferSize int
}

func NewNativeTransferer(bufferSize int) *NativeTransferer {
	if bufferSize <= 0 {
		bufferSize = 32 * 1024 * 1024
	}
	return &NativeTransferer{bufferSize: bufferSize}
}

func (n *NativeTransferer) Name() string {
	return "native"
}

func (n *NativeTransferer) CanResume() bool {
	return false
}

func (n *NativeTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	result, err := n.Copy(src, dst, opts)
	if err != nil {
		return result, err
	}

	if err := RemoveWithTimeout(src, 30*time.Second); err != nil {
		result.SourceRemoved = false
		return result, nil
	}

	result.SourceRemoved = true
	return result, nil
}

func (n *NativeTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	result := &TransferResult{Attempts: 0}
	startTime := time.Now()

	srcInfo, err := StatWithTimeout(src, 10*time.Second)
	if err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrSourceNotFound, err)
		return result, result.Error
	}
	result.BytesTotal = srcInfo.Size()

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrDestinationNotWritable, err)
		return result, result.Error
	}

	if err := CheckDiskHealthForTransfer(src, dst, 5*time.Second, result.BytesTotal); err != nil {
		result.Error = fmt.Errorf("%w: %v", ErrDiskUnhealthy, err)
		return result, result.Error
	}

	maxAttempts := opts.RetryAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt

		bytesCopied, err := n.copyFile(src, dst, srcInfo.Size(), opts)
		if err == nil {
			result.Success = true
			result.BytesCopied = bytesCopied
			result.Duration = time.Since(startTime)
			return result, nil
		}

		lastErr = err

		if opts.DeletePartial {
			os.Remove(dst)
		}

		if attempt < maxAttempts {
			time.Sleep(opts.RetryDelay)
		}
	}

	result.Error = fmt.Errorf("%w: %v", ErrRetryExhausted, lastErr)
	result.Duration = time.Since(startTime)
	return result, result.Error
}

func (n *NativeTransferer) copyFile(src, dst string, totalSize int64, opts TransferOptions) (int64, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srcFile, err := OpenWithTimeout(src, os.O_RDONLY, 0, 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := OpenWithTimeout(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644, 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	var bytesCopied int64
	var lastProgress atomic.Int64
	lastProgress.Store(time.Now().UnixNano())

	go func() {
		ticker := time.NewTicker(timeout / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				lastTime := time.Unix(0, lastProgress.Load())
				if time.Since(lastTime) > timeout {
					cancel()
					return
				}
			}
		}
	}()

	buf := make([]byte, n.bufferSize)

	for {
		select {
		case <-ctx.Done():
			return bytesCopied, fmt.Errorf("%w: no progress for %s", ErrTimeout, timeout)
		default:
		}

		nr, readErr := srcFile.Read(buf)
		if nr > 0 {
			nw, writeErr := dstFile.Write(buf[:nr])
			if nw > 0 {
				bytesCopied += int64(nw)
				lastProgress.Store(time.Now().UnixNano())

				if opts.Progress != nil {
					opts.Progress(bytesCopied, totalSize)
				}
			}
			if writeErr != nil {
				return bytesCopied, fmt.Errorf("write error: %w", writeErr)
			}
			if nr != nw {
				return bytesCopied, fmt.Errorf("short write: %d != %d", nr, nw)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return bytesCopied, fmt.Errorf("read error: %w", readErr)
		}
	}

	if err := dstFile.Sync(); err != nil {
		return bytesCopied, fmt.Errorf("sync error: %w", err)
	}

	if err := applyPermissions(dst, opts); err != nil {
		return bytesCopied, fmt.Errorf("permission error: %w", err)
	}

	return bytesCopied, nil
}

func applyPermissions(path string, opts TransferOptions) error {
	if opts.FileMode != 0 {
		if err := os.Chmod(path, opts.FileMode); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	if opts.TargetUID >= 0 || opts.TargetGID >= 0 {
		uid := opts.TargetUID
		gid := opts.TargetGID
		if uid < 0 {
			uid = -1
		}
		if gid < 0 {
			gid = -1
		}
		if err := os.Chown(path, uid, gid); err != nil {
			if os.Geteuid() != 0 {
				return fmt.Errorf("chown failed (daemon not running as root): target uid=%d gid=%d, current euid=%d: %w",
					uid, gid, os.Geteuid(), err)
			}
			return fmt.Errorf("chown failed: %w", err)
		}
	}

	return nil
}
