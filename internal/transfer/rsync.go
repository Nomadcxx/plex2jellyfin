package transfer

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RsyncTransferer struct {
	rsyncPath string
}

func NewRsyncTransferer(rsyncPath string) *RsyncTransferer {
	return &RsyncTransferer{rsyncPath: rsyncPath}
}

func (r *RsyncTransferer) Name() string {
	return "rsync"
}

func (r *RsyncTransferer) CanResume() bool {
	return true
}

func (r *RsyncTransferer) Move(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return r.transfer(src, dst, opts, true)
}

func (r *RsyncTransferer) Copy(src, dst string, opts TransferOptions) (*TransferResult, error) {
	return r.transfer(src, dst, opts, false)
}

func (r *RsyncTransferer) transfer(src, dst string, opts TransferOptions, removeSource bool) (*TransferResult, error) {
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

	if !opts.SkipHealthCheck {
		if err := CheckDiskHealthForTransfer(src, dst, 30*time.Second, result.BytesTotal); err != nil {
			result.Error = fmt.Errorf("%w: %v", ErrDiskUnhealthy, err)
			return result, result.Error
		}
	}

	maxAttempts := opts.RetryAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	stagedDst := r.stagingPath(src, dst)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt

		err := r.runRsync(src, stagedDst, opts)
		if err == nil {
			if err := os.Rename(stagedDst, dst); err != nil {
				lastErr = fmt.Errorf("publishing completed transfer: %w", err)
			} else {
				if removeSource {
					if err := RemoveWithTimeout(src, 30*time.Second); err != nil {
						result.Error = fmt.Errorf("transfer published but failed to remove source: %w", err)
						result.Duration = time.Since(startTime)
						return result, result.Error
					}
					result.SourceRemoved = true
				}
				result.Success = true
				result.BytesCopied = result.BytesTotal
				result.Duration = time.Since(startTime)
				return result, nil
			}
		} else {
			lastErr = err
		}

		if attempt < maxAttempts {
			time.Sleep(opts.RetryDelay)
		}
	}

	result.Error = fmt.Errorf("%w: %v", ErrRetryExhausted, lastErr)
	result.Duration = time.Since(startTime)
	return result, result.Error
}

func (r *RsyncTransferer) stagingPath(src, dst string) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(filepath.Clean(src)))
	return filepath.Join(filepath.Dir(dst), fmt.Sprintf(".%s.p2j-partial-%x", filepath.Base(dst), hash.Sum64()))
}

func (r *RsyncTransferer) runRsync(src, dst string, opts TransferOptions) error {
	args := r.buildArgs(opts)
	args = append(args, src, dst)

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout*2)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.rsyncPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start rsync: %w", err)
	}

	stdoutReader := bufio.NewReader(stdout)
	if opts.Progress != nil {
		go r.parseProgress(stdoutReader, opts.Progress)
	} else {
		go func() {
			scanner := bufio.NewScanner(stdoutReader)
			for scanner.Scan() {
			}
		}()
	}

	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrOutput.WriteString(scanner.Text())
			stderrOutput.WriteString("\n")
		}
	}()

	err = cmd.Wait()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%w: rsync exceeded timeout of %s", ErrTimeout, timeout*2)
		}
		errMsg := stderrOutput.String()
		if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "I/O error") {
			return fmt.Errorf("%w: %s", ErrTimeout, errMsg)
		}
		return fmt.Errorf("rsync failed: %w (stderr: %s)", err, errMsg)
	}

	return nil
}

func (r *RsyncTransferer) buildArgs(opts TransferOptions) []string {
	args := []string{
		"--progress",
		"--partial",
		"-a",
	}

	if opts.Timeout > 0 {
		timeoutSec := int(opts.Timeout.Seconds())
		if timeoutSec < 10 {
			timeoutSec = 10
		}
		args = append(args, fmt.Sprintf("--timeout=%d", timeoutSec))
	}

	if opts.Checksum {
		args = append(args, "--checksum")
	}

	if !opts.PreserveAttrs {
		args = append(args, "--no-owner", "--no-group", "--no-perms")
	}

	if !opts.PreserveTimes {
		args = append(args, "--no-times")
	}

	if opts.TargetUID >= 0 || opts.TargetGID >= 0 {
		uid := opts.TargetUID
		gid := opts.TargetGID

		if uid >= 0 && gid >= 0 {
			args = append(args, fmt.Sprintf("--chown=%d:%d", uid, gid))
		} else if uid >= 0 {
			args = append(args, fmt.Sprintf("--chown=%d", uid))
		} else if gid >= 0 {
			args = append(args, fmt.Sprintf("--chown=:%d", gid))
		}
	}

	if opts.FileMode != 0 || opts.DirMode != 0 {
		var chmodParts []string
		if opts.DirMode != 0 {
			chmodParts = append(chmodParts, fmt.Sprintf("D%04o", opts.DirMode))
		}
		if opts.FileMode != 0 {
			chmodParts = append(chmodParts, fmt.Sprintf("F%04o", opts.FileMode))
		}
		args = append(args, "--chmod="+strings.Join(chmodParts, ","))
	}

	return args
}

var rsyncProgressRegex = regexp.MustCompile(`^\s*(\d[\d,]*)\s+(\d+)%`)

func (r *RsyncTransferer) parseProgress(stdout *bufio.Reader, progressFn func(current, total int64)) {
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanPVOutput)

	for scanner.Scan() {
		line := scanner.Text()
		matches := rsyncProgressRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			bytesStr := strings.ReplaceAll(matches[1], ",", "")
			bytes, err := strconv.ParseInt(bytesStr, 10, 64)
			if err == nil {
				progressFn(bytes, -1)
			}
		}
	}
}
