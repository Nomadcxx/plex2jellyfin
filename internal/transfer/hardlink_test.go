package transfer

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type hardlinkFallbackStub struct {
	copyCalls     int
	copySource    string
	copyAsSymlink bool
	moveCalls     int
}

func (s *hardlinkFallbackStub) Move(src, dst string, _ TransferOptions) (*TransferResult, error) {
	s.moveCalls++
	return &TransferResult{Success: true, SourceRemoved: true}, nil
}

func (s *hardlinkFallbackStub) Copy(src, dst string, _ TransferOptions) (*TransferResult, error) {
	s.copyCalls++
	s.copySource = src
	if s.copyAsSymlink {
		if err := os.Symlink(src, dst); err != nil {
			return &TransferResult{Error: err}, err
		}
		return &TransferResult{Success: true}, nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return &TransferResult{Error: err}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return &TransferResult{Error: err}, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return &TransferResult{Error: err}, err
	}
	return &TransferResult{Success: true, BytesTotal: int64(len(data)), BytesCopied: int64(len(data))}, nil
}

func (s *hardlinkFallbackStub) CanResume() bool { return true }
func (s *hardlinkFallbackStub) Name() string    { return "stub" }

func TestHardlinkCopyCreatesSharedInodeAndRetainsSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "episode.mkv")
	data := []byte("completed torrent payload")
	if err := os.WriteFile(src, data, 0o640); err != nil {
		t.Fatal(err)
	}

	result, err := NewHardlinkCopyTransferer(&hardlinkFallbackStub{}).Copy(src, dst, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcInfo, dstInfo) {
		t.Fatal("source and target are not hardlinks to the same inode")
	}
	if result.BytesTotal != int64(len(data)) || result.BytesCopied != 0 || result.SourceRemoved {
		t.Fatalf("unexpected result: %+v", result)
	}
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(src); err != nil || string(got) != string(data) {
		t.Fatalf("source changed after target removal: %q, %v", got, err)
	}
}

func TestHardlinkCopyFallsBackForSymlinkSource(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.mkv")
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "episode.mkv")
	if err := os.WriteFile(target, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, src); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	fallback := &hardlinkFallbackStub{}
	result, err := NewHardlinkCopyTransferer(fallback).Copy(src, dst, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("result = %+v, want successful fallback copy", result)
	}
	if fallback.copyCalls != 1 {
		t.Fatalf("fallback copy calls = %d, want 1", fallback.copyCalls)
	}
	if fallback.copySource != target {
		t.Fatalf("fallback source = %q, want resolved target %q", fallback.copySource, target)
	}
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("destination mode = %v, want a regular file", info.Mode())
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "payload" {
		t.Fatalf("destination content = %q, %v, want copied payload", got, err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(info, targetInfo) {
		t.Fatal("symlink source fallback unexpectedly published a hardlink")
	}
}

func TestHardlinkCopyRejectsSymlinkToNonRegularSource(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target-dir")
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "episode.mkv")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, src); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	fallback := &hardlinkFallbackStub{}
	_, err := NewHardlinkCopyTransferer(fallback).Copy(src, dst, DefaultOptions())
	if !errors.Is(err, ErrSourceNotRegular) {
		t.Fatalf("error = %v, want ErrSourceNotRegular", err)
	}
	if fallback.copyCalls != 0 {
		t.Fatalf("fallback copy calls = %d, want 0", fallback.copyCalls)
	}
}

func TestHardlinkCopyDoesNotPublishFallbackSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.mkv")
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "episode.mkv")
	if err := os.WriteFile(target, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, src); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	fallback := &hardlinkFallbackStub{copyAsSymlink: true}
	_, err := NewHardlinkCopyTransferer(fallback).Copy(src, dst, DefaultOptions())
	if !errors.Is(err, ErrSourceNotRegular) {
		t.Fatalf("error = %v, want ErrSourceNotRegular", err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("destination exists or cannot be inspected: %v", err)
	}
}

func TestHardlinkCopyFallsBackAcrossDevices(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	dst := filepath.Join(dir, "library", "episode.mkv")
	if err := os.WriteFile(src, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	fallback := &hardlinkFallbackStub{}
	transferer := NewHardlinkCopyTransferer(fallback)
	transferer.link = func(_, _ string) error { return syscall.EXDEV }

	result, err := transferer.Copy(src, dst, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if fallback.copyCalls != 1 || !result.Success {
		t.Fatalf("fallback calls=%d result=%+v", fallback.copyCalls, result)
	}
	if srcInfo, err := os.Stat(src); err != nil {
		t.Fatal(err)
	} else if dstInfo, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	} else if os.SameFile(srcInfo, dstInfo) {
		t.Fatal("cross-device fallback unexpectedly produced a hardlink")
	}
}

func TestHardlinkCopyReturnsUnexpectedLinkError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	if err := os.WriteFile(src, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	fallback := &hardlinkFallbackStub{}
	transferer := NewHardlinkCopyTransferer(fallback)
	want := syscall.EACCES
	transferer.link = func(_, _ string) error { return want }

	_, err := transferer.Copy(src, filepath.Join(dir, "target.mkv"), DefaultOptions())
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if fallback.copyCalls != 0 {
		t.Fatalf("unexpected fallback calls: %d", fallback.copyCalls)
	}
}

func TestHardlinkMoveDelegates(t *testing.T) {
	fallback := &hardlinkFallbackStub{}
	transferer := NewHardlinkCopyTransferer(fallback)
	result, err := transferer.Move("source", "target", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if fallback.moveCalls != 1 || !result.SourceRemoved {
		t.Fatalf("move calls=%d result=%+v", fallback.moveCalls, result)
	}
}
