package api

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type failingReloadIPC struct{}

func (f failingReloadIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":false,"failed":[{"name":"ai","error":"bad config"}]}`), nil
}

func (f failingReloadIPC) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return nil
}

type successfulReloadIPC struct{}

func (s successfulReloadIPC) Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true,"reloaded":["logging"]}`), nil
}

func (s successfulReloadIPC) StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error {
	return nil
}

func TestSavePipelineRestoresPreviousConfigOnReloadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	oldCfg := config.DefaultConfig()
	oldCfg.Logging.Level = "info"
	if err := config.AtomicWriteWithLock(path, []byte(oldCfg.ToTOML()), 0600); err != nil {
		t.Fatal(err)
	}

	newCfg := config.DefaultConfig()
	newCfg.Logging.Level = "debug"
	resp, err := SaveConfigAndReload(context.Background(), path, newCfg, failingReloadIPC{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.RestoredPreviousConfig {
		t.Fatalf("expected restore response: %+v", resp)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`level = "info"`)) {
		t.Fatalf("previous config was not restored:\n%s", string(got))
	}
}

func TestSavePipelineKeepsCandidateOnReloadSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	oldCfg := config.DefaultConfig()
	oldCfg.Logging.Level = "info"
	if err := config.AtomicWriteWithLock(path, []byte(oldCfg.ToTOML()), 0600); err != nil {
		t.Fatal(err)
	}

	newCfg := config.DefaultConfig()
	newCfg.Logging.Level = "debug"
	resp, err := SaveConfigAndReload(context.Background(), path, newCfg, successfulReloadIPC{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RestoredPreviousConfig {
		t.Fatalf("did not expect restore response: %+v", resp)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`level = "debug"`)) {
		t.Fatalf("candidate config was not retained:\n%s", string(got))
	}
}
