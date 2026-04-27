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

func TestClientStreamReceivesFrames(t *testing.T) {
dir := t.TempDir()
sock := filepath.Join(dir, "ctl.sock")
srv := NewServer(sock)
srv.SetRegistry(NewOpRegistry())
srv.RegisterStreaming(Command("FAKE"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
w.Progress(op.ID, "p1", "hello", 1, 10)
w.Progress(op.ID, "p1", "world", 2, 10)
w.Done(op.ID, json.RawMessage(`{"ok":true}`))
})

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
if err := srv.Start(ctx); err != nil {
t.Fatal(err)
}
defer srv.Stop()

cli := NewClient(sock)
frames, errc := cli.Stream(ctx, Command("FAKE"), nil)
gotProgress := 0
gotDone := false
for f := range frames {
switch f.Type {
case FrameProgress:
gotProgress++
case FrameDone:
gotDone = true
}
}
if err := <-errc; err != nil {
t.Fatal(err)
}
if gotProgress != 2 || !gotDone {
t.Errorf("got %d progress, done=%v", gotProgress, gotDone)
}
}
