package ipc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestClientCallStatus(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli := NewClient(sock)
	res, err := cli.Call(ctx, CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatal(err)
	}
	if got["state"] != "running" {
		t.Errorf("state = %q", got["state"])
	}
}
