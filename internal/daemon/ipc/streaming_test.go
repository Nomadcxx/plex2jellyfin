package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
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

func TestStreamingHandlerPanicCleansUpHeartbeat(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.SetRegistry(NewOpRegistry())

	panicCh := make(chan struct{})
	srv.RegisterStreaming(Command("PANIC"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
		close(panicCh)
		panic("test panic in streaming handler")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, _ := net.Dial("unix", sock)
	defer c.Close()
	c.Write([]byte(`{"v":1,"id":"op-1","cmd":"PANIC"}` + "\n"))

	select {
	case <-panicCh:
	case <-time.After(2 * time.Second):
		t.Fatal("streaming handler never executed")
	}

	time.Sleep(100 * time.Millisecond)

	op, ok := srv.registry.Get("op-1")
	if !ok {
		t.Fatal("op not found in registry after panic")
	}
	if op.Final == nil {
		t.Error("op should be finalized after handler panic")
	}
}

func TestServerRequestTimeout(t *testing.T) {
	requestTimeout = 100 * time.Millisecond
	defer func() { requestTimeout = 30 * time.Second }()

	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)

	done := make(chan struct{})
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		<-ctx.Done()
		close(done)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, _ := net.Dial("unix", sock)
	defer c.Close()
	c.Write([]byte(`{"v":1,"id":"r1","cmd":"STATUS"}` + "\n"))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler context was not cancelled within timeout")
	}
}

func TestServerRequestSemaphoreRejectsOverflow(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)

	blockCh := make(chan struct{})
	srv.Register(CmdStatus, func(ctx context.Context, req Request, w FrameWriter) {
		<-blockCh
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, _ := net.Dial("unix", sock)
	defer c.Close()

	for i := 0; i < maxConcurrentPerConn+1; i++ {
		c.Write([]byte(`{"v":1,"id":"r` + strconv.Itoa(i) + `","cmd":"STATUS"}` + "\n"))
	}

	time.Sleep(200 * time.Millisecond)

	dec := json.NewDecoder(c)
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	var rejected bool
	for i := 0; i < maxConcurrentPerConn+1; i++ {
		var f Frame
		if err := dec.Decode(&f); err != nil {
			break
		}
		if f.Type == FrameError && f.Code == ErrBusy {
			rejected = true
		}
	}
	if !rejected {
		t.Error("expected at least one request to be rejected by semaphore")
	}

	close(blockCh)
}

func TestStreamingHandlerOutlivesRequestTimeout(t *testing.T) {
	old := requestTimeout
	requestTimeout = 50 * time.Millisecond
	defer func() { requestTimeout = old }()

	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := NewServer(sock)
	srv.SetRegistry(NewOpRegistry())

	srv.RegisterStreaming(Command("SLOW"), func(ctx context.Context, args json.RawMessage, w FrameWriter, op *Op) {
		select {
		case <-time.After(300 * time.Millisecond):
			w.Done(op.ID, json.RawMessage(`{"ok":true}`))
		case <-ctx.Done():
			w.Error(op.ID, ErrInternal, ctx.Err().Error())
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	c, _ := net.Dial("unix", sock)
	defer c.Close()
	c.Write([]byte(`{"v":1,"id":"op-slow","cmd":"SLOW"}` + "\n"))
	dec := json.NewDecoder(c)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var f Frame
		if err := dec.Decode(&f); err != nil {
			t.Fatal(err)
		}
		if f.Type == FrameDone {
			return
		}
		if f.Type == FrameError {
			t.Fatalf("streaming op killed by request timeout: %+v", f)
		}
	}
}
