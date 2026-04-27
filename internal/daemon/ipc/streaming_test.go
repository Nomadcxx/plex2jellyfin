package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestAttachReplaysFrames(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.SetRegistry(NewOpRegistry())

	srv.RegisterStreaming(Command("FAKE"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
		op.Frames.Append(Frame{ID: op.ID, Type: FrameProgress, Phase: "p1", Msg: "hello"})
		if rw, ok := w.(*ringWriter); ok {
			rw.write(Frame{ID: op.ID, Type: FrameProgress, Phase: "p1", Msg: "hello"})
		}
		<-ctx.Done()
	})

	srv.Register(CmdAttach, AttachHandler(srv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c1, _ := net.Dial("unix", sock)
	defer c1.Close()
	c1.Write([]byte(`{"v":1,"id":"op-1","cmd":"FAKE"}` + "\n"))
	dec1 := json.NewDecoder(c1)
	c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	var f1 Frame
	if err := dec1.Decode(&f1); err != nil {
		t.Fatal(err)
	}
	if f1.Phase != "p1" {
		t.Errorf("first frame: %+v", f1)
	}

	c2, _ := net.Dial("unix", sock)
	defer c2.Close()
	c2.Write([]byte(`{"v":1,"id":"r2","cmd":"ATTACH","args":{"op_id":"op-1"}}` + "\n"))
	dec2 := json.NewDecoder(c2)
	c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	var f2 Frame
	if err := dec2.Decode(&f2); err != nil {
		t.Fatal(err)
	}
	if f2.Phase != "p1" || f2.ID != "op-1" {
		t.Errorf("replay frame: %+v", f2)
	}
}
