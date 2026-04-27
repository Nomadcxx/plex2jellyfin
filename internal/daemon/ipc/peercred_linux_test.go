//go:build linux

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestPeerUIDMatchesSelf(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		c, _ := net.Dial("unix", sockPath)
		if c != nil {
			defer c.Close()
		}
	}()

	conn, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	uid, err := peerUID(conn.(*net.UnixConn))
	if err != nil {
		t.Fatal(err)
	}
	if uid != os.Getuid() {
		t.Errorf("peer UID %d != self UID %d", uid, os.Getuid())
	}
}
