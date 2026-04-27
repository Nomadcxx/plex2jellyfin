package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerHandlesStatus(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "control.sock")

	srv := NewServer(sockPath)
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.Write([]byte(`{"v":1,"id":"r1","cmd":"STATUS"}` + "\n")); err != nil {
		t.Fatal(err)
	}

	dec := json.NewDecoder(c)
	var f Frame
	if err := dec.Decode(&f); err != nil {
		t.Fatal(err)
	}
	if f.ID != "r1" || f.Type != FrameResult {
		t.Errorf("unexpected frame: %+v", f)
	}
}

func TestServerAllowedPeerUIDs(t *testing.T) {
	srv := NewServer(filepath.Join(t.TempDir(), "control.sock"))
	if !srv.peerAllowed(os.Getuid()) {
		t.Fatal("current UID should be allowed by default")
	}
	srv.AddAllowedPeerUID(123456)
	if !srv.peerAllowed(123456) {
		t.Fatal("explicitly allowed UID was not accepted")
	}
}
