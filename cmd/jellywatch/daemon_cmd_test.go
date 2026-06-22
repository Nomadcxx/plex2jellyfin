package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

func TestDaemonStatusCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := ipc.NewServer(sock)
	srv.Register(ipc.CmdStatus, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"state":"running"}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	var buf bytes.Buffer
	cmd := newDaemonStatusCmd(sock, &buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"state":"running"`)) {
		t.Errorf("output: %s", buf.String())
	}
}

func TestDaemonReloadCommandPrintsJSON(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := ipc.NewServer(sock)
	srv.Register(ipc.CmdReload, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		w.Result(req.ID, json.RawMessage(`{"ok":true}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	var buf bytes.Buffer
	cmd := newDaemonReloadCmd(sock, &buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"ok":true`)) {
		t.Errorf("output: %s", buf.String())
	}
}

func TestDaemonStopCommandCallsStop(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	srv := ipc.NewServer(sock)
	called := make(chan struct{}, 1)
	srv.Register(ipc.CmdStop, func(ctx context.Context, req ipc.Request, w ipc.FrameWriter) {
		called <- struct{}{}
		w.Result(req.ID, json.RawMessage(`{"stopping":true}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	var buf bytes.Buffer
	cmd := newDaemonStopCmd(sock, &buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	default:
		t.Fatal("stop handler not invoked")
	}
}

func TestDaemonStatusCommandReportsActionableMissingDaemon(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "missing.sock")
	var buf bytes.Buffer
	cmd := newDaemonStatusCmd(sock, &buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing daemon socket")
	}
	msg := err.Error()
	if !strings.Contains(msg, "jellywatchd does not appear to be running") {
		t.Fatalf("expected actionable daemon message, got %q", msg)
	}
	if strings.Contains(msg, "dial unix") {
		t.Fatalf("expected raw dial error to be wrapped, got %q", msg)
	}
}
