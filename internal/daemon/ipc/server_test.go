package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
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

func TestServerAppliesSocketOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("socket owner test requires root")
	}
	u, err := user.Lookup("nomadx")
	if err != nil {
		t.Skip("test host does not have nomadx user")
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	sockPath := filepath.Join(dir, "control.sock")
	srv := NewServer(sockPath)
	srv.SetSocketOwner(uid, gid)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	info, err := os.Stat(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("socket stat did not expose syscall.Stat_t")
	}
	if int(st.Uid) != uid || int(st.Gid) != gid {
		t.Fatalf("socket owner = %d:%d, want %d:%d", st.Uid, st.Gid, uid, gid)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %o, want 0600", info.Mode().Perm())
	}
}
