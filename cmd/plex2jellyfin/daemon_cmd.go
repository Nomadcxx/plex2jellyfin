package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/daemon/ipc"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/spf13/cobra"
)

func newDaemonCmd() *cobra.Command {
	c := &cobra.Command{Use: "daemon", Short: "Daemon control"}
	c.AddCommand(newDaemonStatusCmd("", os.Stdout))
	c.AddCommand(newDaemonReloadCmd("", os.Stdout))
	c.AddCommand(newDaemonStopCmd("", os.Stdout))
	return c
}

func socketPath() string {
	d, _ := paths.Plex2JellyfinDir()
	return filepath.Join(d, "control.sock")
}

func newDaemonStatusCmd(sock string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := sock
			if path == "" {
				path = socketPath()
			}
			cli := ipc.NewClient(path)
			body, err := cli.Call(context.Background(), ipc.CmdStatus, nil)
			if err != nil {
				return formatDaemonIPCError(err, path)
			}
			fmt.Fprintln(out, string(body))
			return nil
		},
	}
}

func formatDaemonIPCError(err error, path string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	var opErr *net.OpError
	if os.IsNotExist(err) || strings.Contains(msg, "no such file or directory") {
		return fmt.Errorf("plex2jellyfin-daemon does not appear to be running: control socket not found at %s", path)
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset by peer") {
		return fmt.Errorf("plex2jellyfin-daemon control socket at %s is stale or unhealthy: %w", path, err)
	}
	if strings.Contains(msg, "i/o timeout") {
		return fmt.Errorf("plex2jellyfin-daemon control socket at %s did not respond before timeout: %w", path, err)
	}
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return fmt.Errorf("unable to contact plex2jellyfin-daemon control socket at %s: %w", path, err)
	}
	return err
}

func newDaemonReloadCmd(sock string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload daemon config",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := sock
			if path == "" {
				path = socketPath()
			}
			cli := ipc.NewClient(path)
			body, err := cli.Call(context.Background(), ipc.CmdReload, nil)
			if err != nil {
				return formatDaemonIPCError(err, path)
			}
			fmt.Fprintln(out, string(body))
			return nil
		},
	}
}

func newDaemonStopCmd(sock string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := sock
			if path == "" {
				path = socketPath()
			}
			cli := ipc.NewClient(path)
			body, err := cli.Call(context.Background(), ipc.CmdStop, nil)
			if err != nil {
				return formatDaemonIPCError(err, path)
			}
			if len(body) > 0 {
				fmt.Fprintln(out, string(body))
			}
			return nil
		},
	}
}
