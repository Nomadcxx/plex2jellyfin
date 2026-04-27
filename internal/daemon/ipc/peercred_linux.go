//go:build linux

package ipc

import (
	"errors"
	"net"
	"syscall"
)

func peerUID(c *net.UnixConn) (int, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	var ucred *syscall.Ucred
	var sockErr error
	if err := rc.Control(func(fd uintptr) {
		ucred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return -1, err
	}
	if sockErr != nil {
		return -1, sockErr
	}
	if ucred == nil {
		return -1, errors.New("peer credentials unavailable")
	}
	return int(ucred.Uid), nil
}
