package transfer

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/fileidentity"
)

func TestTransferResultValidate(t *testing.T) {
	transferErr := errors.New("copy failed")
	tests := []struct {
		name   string
		result *TransferResult
		want   error
	}{
		{name: "nil result", want: ErrTransferFailed},
		{name: "unsuccessful without error", result: &TransferResult{}, want: ErrTransferFailed},
		{name: "unsuccessful with error", result: &TransferResult{Error: transferErr}, want: transferErr},
		{name: "success", result: &TransferResult{Success: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want error matching %v", err, tt.want)
			}
		})
	}
}

func TestRenameMoveRestoresChangedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, ok := fileidentity.FromLstat(source)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	quarantine, err := fileidentity.NewQuarantinePath(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}

	called := false
	result, err := RenameMove(source, destination, TransferOptions{
		MoveSourceIdentity:       identity,
		MoveSourceQuarantinePath: quarantine,
		OnPublished: func(string, string) error {
			called = true
			return nil
		},
	})
	if err == nil || result != nil {
		t.Fatalf("RenameMove() = (%#v, %v), want failure before publication", result, err)
	}
	if called {
		t.Fatal("published callback ran for a changed source")
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("source replacement = %q, error = %v", got, err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination after rejected move: %v", err)
	}
	if _, err := os.Lstat(quarantine); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source quarantine after rejected move: %v", err)
	}
}

func TestRenameMovePublishesCapturedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, ok := fileidentity.FromLstat(source)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	quarantine, err := fileidentity.NewQuarantinePath(source)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	result, err := RenameMove(source, destination, TransferOptions{
		MoveSourceIdentity:       identity,
		MoveSourceQuarantinePath: quarantine,
		OnPublished: func(gotIdentity, _ string) error {
			called = true
			if gotIdentity != identity {
				return errors.New("unexpected published identity")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("RenameMove(): %v", err)
	}
	if err := result.Validate(); err != nil || !result.SourceRemoved || !result.Published || result.PublishedIdentity != identity {
		t.Fatalf("RenameMove result = %#v, validation error = %v", result, err)
	}
	if !called {
		t.Fatal("published callback was not called")
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source after move: %v", err)
	}
	if !fileidentity.MatchesLstat(destination, identity) {
		t.Fatal("destination does not have the captured source identity")
	}
}

func TestRollbackMoveDoesNotReplaceARecreatedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("new source"), 0o644); err != nil {
		t.Fatalf("create recreated source: %v", err)
	}
	if err := os.WriteFile(destination, []byte("moved media"), 0o644); err != nil {
		t.Fatalf("create moved destination: %v", err)
	}

	err := RollbackMove(NewNativeTransferer(1024), source, destination, DefaultOptions(), "original")
	if err == nil {
		t.Fatal("RollbackMove succeeded despite a recreated source path")
	}
	for path, want := range map[string]string{source: "new source", destination: "moved media"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, error = %v; want %q", path, got, err, want)
		}
	}
}

func TestRollbackMoveDoesNotMoveReplacementDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(destination, []byte("moved media"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, ok := fileidentity.FromPath(destination)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("unrelated replacement"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RollbackMove(NewNativeTransferer(1024), source, destination, DefaultOptions(), identity)
	if err == nil {
		t.Fatal("RollbackMove moved a replacement destination")
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source created by rollback: stat error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "unrelated replacement" {
		t.Fatalf("destination = %q, error = %v", got, err)
	}
}

func TestNativeMoveDoesNotReplaceExistingDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("source media"), 0o644); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := os.WriteFile(destination, []byte("keep this media"), 0o644); err != nil {
		t.Fatalf("create existing destination: %v", err)
	}

	_, err := NewNativeTransferer(1024).Move(source, destination, TransferOptions{
		SkipHealthCheck: true,
		TargetUID:       -1,
		TargetGID:       -1,
	})
	if err == nil {
		t.Fatal("Move replaced an existing destination")
	}
	for path, want := range map[string]string{source: "source media", destination: "keep this media"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, error = %v; want %q", path, got, err, want)
		}
	}
}

func TestNativeMoveRecordsPublishedFileBeforeRemovingSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("source media"), 0o644); err != nil {
		t.Fatal(err)
	}
	var publishedIdentity string
	opts := TransferOptions{
		SkipHealthCheck: true,
		TargetUID:       -1,
		TargetGID:       -1,
		OnPublished: func(identity, _ string) error {
			if _, err := os.Stat(source); err != nil {
				return errors.New("source was removed before publication was recorded")
			}
			if identity == "" {
				return errors.New("published destination has no stable identity")
			}
			if !fileidentity.MatchesPath(destination, identity) {
				return errors.New("published identity does not match destination")
			}
			publishedIdentity = identity
			return nil
		},
	}

	result, err := NewNativeTransferer(1024).Move(source, destination, opts)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if result == nil || !result.Published || !result.SourceRemoved || result.PublishedIdentity != publishedIdentity {
		t.Fatalf("Move result = %+v, published identity = %q", result, publishedIdentity)
	}
}

func TestNativeMoveDoesNotRemoveSourceWhenPublicationRecordFails(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("source media"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordErr := errors.New("database unavailable")
	result, err := NewNativeTransferer(1024).Move(source, destination, TransferOptions{
		SkipHealthCheck: true,
		TargetUID:       -1,
		TargetGID:       -1,
		OnPublished:     func(string, string) error { return recordErr },
	})
	if !errors.Is(err, recordErr) {
		t.Fatalf("Move error = %v, want %v", err, recordErr)
	}
	if result == nil || !result.Published || result.PublishedIdentity == "" || result.SourceRemoved {
		t.Fatalf("Move result = %+v, want published but source retained", result)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source file after publication record failure: %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination remains after successful cleanup: stat error = %v", err)
	}
}

func TestNativeMovePreservesSourceReplacementCreatedDuringTransfer(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("original media"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewNativeTransferer(1024).Move(source, destination, TransferOptions{
		SkipHealthCheck: true,
		TargetUID:       -1,
		TargetGID:       -1,
		OnPublished: func(string, _ string) error {
			if err := os.Remove(source); err != nil {
				return err
			}
			return os.WriteFile(source, []byte("replacement media"), 0o644)
		},
	})
	if err == nil {
		t.Fatalf("Move result = %+v, want an error after source replacement", result)
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "replacement media" {
		t.Fatalf("source replacement = %q, error = %v", got, err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published destination remains after source replacement: %v", err)
	}
}

func TestNativeMovePreservesSourceWhenPublishedDestinationIsReplaced(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("original media"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewNativeTransferer(1024).Move(source, destination, TransferOptions{
		SkipHealthCheck: true,
		TargetUID:       -1,
		TargetGID:       -1,
		OnPublished: func(string, _ string) error {
			if err := os.Remove(destination); err != nil {
				return err
			}
			return os.WriteFile(destination, []byte("replacement media"), 0o644)
		},
	})
	if err == nil {
		t.Fatalf("Move result = %+v, want destination identity error", result)
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "original media" {
		t.Fatalf("source media = %q, error = %v", got, err)
	}
	got, err = os.ReadFile(destination)
	if err != nil || string(got) != "replacement media" {
		t.Fatalf("destination replacement = %q, error = %v", got, err)
	}
}

func TestRecordPublishedDestinationRejectsDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	result := &TransferResult{}
	if _, err := recordPublishedDestination(result, source, destination, TransferOptions{}); err == nil {
		t.Fatal("recordPublishedDestination accepted a directory")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was removed while rejecting a directory destination: %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty invalid destination remains: %v", err)
	}
}

func TestResolvePendingSourceRemovalAcceptsSymlinkDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	backing := filepath.Join(root, "backing.mkv")
	destination := filepath.Join(root, "destination.mkv")
	if err := os.WriteFile(backing, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backing, destination); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	publishedInfo, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	publishedIdentity, ok := fileidentity.FromLstat(destination)
	if !ok {
		t.Skip("platform does not expose stable file identity")
	}
	pending := make(chan error, 1)
	resolved := resolvePendingSourceRemoval(source, destination, publishedInfo, publishedIdentity, pending)
	pending <- nil
	select {
	case err := <-resolved:
		if err != nil {
			t.Fatalf("resolvePendingSourceRemoval() = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending source removal did not resolve")
	}
}

func TestMoveBackendsRemovePublishedDestinationWhenSourceRemovalFails(t *testing.T) {
	backends := []struct {
		name       string
		transferer Transferer
	}{
		{name: "native", transferer: NewNativeTransferer(1024)},
	}
	if pvPath, err := exec.LookPath("pv"); err == nil {
		backends = append(backends, struct {
			name       string
			transferer Transferer
		}{name: "pv", transferer: NewPVTransferer(pvPath)})
	}
	if rsyncPath, err := exec.LookPath("rsync"); err == nil {
		backends = append(backends, struct {
			name       string
			transferer Transferer
		}{name: "rsync", transferer: NewRsyncTransferer(rsyncPath)})
	}

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			root := t.TempDir()
			sourceDir := filepath.Join(root, "source")
			destinationDir := filepath.Join(root, "library")
			if err := os.Mkdir(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(destinationDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(sourceDir, "source.mkv")
			destination := filepath.Join(destinationDir, "destination.mkv")
			if err := os.WriteFile(source, []byte("source media"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(sourceDir, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o755) })

			opts := DefaultOptions()
			opts.SkipHealthCheck = true
			opts.RetryAttempts = 0
			result, err := backend.transferer.Move(source, destination, opts)
			if err == nil {
				t.Fatal("Move succeeded despite source removal being denied")
			}
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("source file was not retained: %v", err)
			}
			if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("published destination remains after failed source removal: stat error = %v", err)
			}
			if result == nil || result.Success || result.SourceRemoved {
				t.Fatalf("Move result = %+v, want failed move with source retained", result)
			}
		})
	}
}

func TestNewTransferer(t *testing.T) {
	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New(BackendAuto) failed: %v", err)
	}
	if tr == nil {
		t.Fatal("New(BackendAuto) returned nil")
	}
	t.Logf("Using backend: %s", tr.Name())
}

func TestRsyncTransfererInterface(t *testing.T) {
	tr := NewRsyncTransferer("/usr/bin/rsync")
	var _ Transferer = tr
	if tr.Name() != "rsync" {
		t.Errorf("expected name 'rsync', got '%s'", tr.Name())
	}
	if !tr.CanResume() {
		t.Error("rsync should support resume")
	}
}

func TestRsyncMoveDoesNotDeleteSourceBeforeAtomicPublish(t *testing.T) {
	tr := NewRsyncTransferer("/usr/bin/rsync")
	if args := tr.buildArgs(DefaultOptions()); slices.Contains(args, "--remove-source-files") {
		t.Fatalf("rsync move deletes the source before Plex2Jellyfin publishes the final name: %v", args)
	}
	dst := filepath.Join("/library", "Movie (2026)", "Movie (2026).mkv")
	staged := tr.stagingPath("/downloads/release-a/movie.mkv", dst)
	if filepath.Ext(staged) == filepath.Ext(dst) || filepath.Dir(staged) != filepath.Dir(dst) {
		t.Fatalf("staging path %q must be non-media in the destination directory", staged)
	}
	if other := tr.stagingPath("/downloads/release-b/movie.mkv", dst); other == staged {
		t.Fatalf("different sources share staging path %q", staged)
	}
}

func TestNativeTransfererInterface(t *testing.T) {
	tr := NewNativeTransferer(1024 * 1024)
	var _ Transferer = tr
	if tr.Name() != "native" {
		t.Errorf("expected name 'native', got '%s'", tr.Name())
	}
	if tr.CanResume() {
		t.Error("native should not support resume")
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5m, got %v", opts.Timeout)
	}
	if opts.RetryAttempts != 3 {
		t.Errorf("expected RetryAttempts 3, got %d", opts.RetryAttempts)
	}
}

func TestCheckDiskHealth(t *testing.T) {
	health, err := CheckDiskHealth(os.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("CheckDiskHealth failed: %v", err)
	}
	if !health.IsHealthy() {
		t.Errorf("temp dir should be healthy: %+v", health)
	}
}

func TestStatWithTimeout(t *testing.T) {
	info, err := StatWithTimeout(os.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatalf("StatWithTimeout failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected temp dir to be a directory")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	testContent := []byte("test content for transfer")
	if err := os.WriteFile(srcPath, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.Timeout = 30 * time.Second

	result, err := tr.Copy(srcPath, dstPath, opts)
	if err != nil {
		t.Fatalf("Copy failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Copy reported failure: %v", result.Error)
	}
	if result.BytesCopied != int64(len(testContent)) {
		t.Errorf("expected %d bytes copied, got %d", len(testContent), result.BytesCopied)
	}

	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != string(testContent) {
		t.Errorf("content mismatch: expected %q, got %q", testContent, dstContent)
	}
}

func TestMoveFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	testContent := []byte("test content for move")
	if err := os.WriteFile(srcPath, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.Timeout = 30 * time.Second

	result, err := tr.Move(srcPath, dstPath, opts)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Move reported failure: %v", result.Error)
	}

	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should be removed after Move")
	}

	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(dstContent) != string(testContent) {
		t.Errorf("content mismatch: expected %q, got %q", testContent, dstContent)
	}
}

func TestSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "nonexistent.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	tr, err := New(BackendAuto)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	opts := DefaultOptions()
	opts.RetryAttempts = 0

	_, err = tr.Copy(srcPath, dstPath, opts)
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}
