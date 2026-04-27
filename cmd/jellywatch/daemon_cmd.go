package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/paths"
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
	d, _ := paths.JellyWatchDir()
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
				return err
			}
			fmt.Fprintln(out, string(body))
			return nil
		},
	}
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
				return err
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
				return err
			}
			if len(body) > 0 {
				fmt.Fprintln(out, string(body))
			}
			return nil
		},
	}
}
