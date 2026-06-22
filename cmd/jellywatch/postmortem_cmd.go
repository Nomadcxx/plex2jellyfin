package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/postmortem"
	"github.com/spf13/cobra"
)

func newPostmortemCmd() *cobra.Command {
	var since time.Duration
	var root string

	cmd := &cobra.Command{
		Use:    "postmortem",
		Hidden: true,
		Short:  "Collect scheduled postmortem evidence bundles",
	}

	collect := &cobra.Command{
		Use:   "collect",
		Short: "Collect a postmortem evidence bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := database.Open()
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			if root == "" {
				root = config.GetReportsPath()
			}

			logDir := ""
			if cfg, err := config.Load(); err == nil && cfg.Logging.File != "" {
				logDir = filepath.Dir(cfg.Logging.File)
			}

			now := time.Now().UTC()
			c := postmortem.Collector{
				DB:        db,
				Root:      root,
				Now:       func() time.Time { return now },
				Since:     now.Add(-since),
				LogDir:    logDir,
				Workspace: "/home/nomadx/Documents/jellywatch",
			}
			bundle, err := c.Collect()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "postmortem bundle: %s\n", bundle.Dir)
			return nil
		},
	}
	collect.Flags().DurationVar(&since, "since", 96*time.Hour, "lookback window")
	collect.Flags().StringVar(&root, "root", "", "report root directory")
	cmd.AddCommand(collect)

	return cmd
}
