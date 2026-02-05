package transfer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

type PVTransferer struct {
	pvPath string
}

func NewPVTransferer(pvPath string) *PVTransferer {
	return &PVTransferer{pvPath: pvPath}
}

func (p *PVTransferer) Name() string {
	return "pv"
}

func (p *PVTransferer) CanResume() bool {
	return false
}

func (p *PVTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	result, err := p.Copy(src, dst, opts)
	if err != nil {
		return result, err
	}

	if result.Success {
		// Apply configured permissions (chown/chmod)
		if err := ApplyPermissions(dst, opts); err != nil {
			// Log warning but don't fail - file was transferred successfully
			result.Error = fmt.Errorf("transfer succeeded but permission application failed: %w", err)
		}

		if err := RemoveWithTimeout(src, 30*time.Second); err != nil {
			result.SourceRemoved = false
			return result, nil
		}
		result.SourceRemoved = true
	}

	return result, nil
}

func (p *PVTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
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

		bytesCopied, err := p.runPV(src, dst, result.BytesTotal, opts)
		if err == nil {
			dstInfo, statErr := StatWithTimeout(dst, 10*time.Second)
			if statErr != nil {
				lastErr = fmt.Errorf("cannot verify destination: %w", statErr)
			} else if dstInfo.Size() != result.BytesTotal {
				lastErr = fmt.Errorf("size mismatch: expected %d, got %d", result.BytesTotal, dstInfo.Size())
			} else {
				result.Success = true
				result.BytesCopied = bytesCopied
				result.Duration = time.Since(startTime)
				return result, nil
			}
		} else {
			lastErr = err
		}

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

func (p *PVTransferer) runPV(src, dst string, totalSize int64, opts TransferOptions) (int64, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	args := p.buildArgs(totalSize)
	args = append(args, src)

	cmd := exec.CommandContext(ctx, p.pvPath, args...)
	cmd.Stdout = dstFile

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start pv: %w", err)
	}

	var lastProgress atomic.Int64
	lastProgress.Store(time.Now().UnixNano())
	var bytesCopied atomic.Int64

	go func() {
		p.parseProgress(stderr, func(current, total int64) {
			bytesCopied.Store(current)
			lastProgress.Store(time.Now().UnixNano())
			if opts.Progress != nil {
				opts.Progress(current, total)
			}
		})
	}()

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
					cmd.Process.Kill()
					return
				}
			}
		}
	}()

	err = cmd.Wait()
	if err != nil {
		if ctx.Err() != nil {
			return bytesCopied.Load(), fmt.Errorf("%w: transfer stalled - no progress for %s", ErrTimeout, timeout)
		}
		return bytesCopied.Load(), fmt.Errorf("pv failed: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return bytesCopied.Load(), fmt.Errorf("sync error: %w", err)
	}

	return totalSize, nil
}

func (p *PVTransferer) buildArgs(totalSize int64) []string {
	args := []string{
		"-p", "-t", "-e", "-r", "-b",
	}

	if totalSize > 0 {
		args = append(args, "-s", fmt.Sprintf("%d", totalSize))
	}

	return args
}

var pvProgressRegex = regexp.MustCompile(`([\d.]+)\s*([KMGT]i?B)`)

func (p *PVTransferer) parseProgress(stderr io.Reader, progressFn func(current, total int64)) {
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanPVOutput)

	for scanner.Scan() {
		line := scanner.Text()

		matches := pvProgressRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			value, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				bytes := convertToBytes(value, matches[2])
				if progressFn != nil {
					progressFn(bytes, -1)
				}
			}
		}
	}
}

func convertToBytes(value float64, unit string) int64 {
	multipliers := map[string]float64{
		"B":   1,
		"KB":  1000,
		"KiB": 1024,
		"MB":  1000 * 1000,
		"MiB": 1024 * 1024,
		"GB":  1000 * 1000 * 1000,
		"GiB": 1024 * 1024 * 1024,
		"TB":  1000 * 1000 * 1000 * 1000,
		"TiB": 1024 * 1024 * 1024 * 1024,
	}
	if mult, ok := multipliers[unit]; ok {
		return int64(value * mult)
	}
	return int64(value)
}

func scanPVOutput(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i := 0; i < len(data); i++ {
		if data[i] == '\r' || data[i] == '\n' {
			return i + 1, data[0:i], nil
		}
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}
