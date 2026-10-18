package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/Nomadcxx/plex2jellyfin/internal/postmortem"
	"github.com/spf13/cobra"
)

const postmortemCadence = 96 * time.Hour

func newPostmortemCmd() *cobra.Command {
	var since time.Duration
	var root string
	var ifDue bool

	cmd := &cobra.Command{
		Use:    "postmortem",
		Hidden: true,
		Short:  "Collect scheduled postmortem evidence bundles",
	}

	collect := &cobra.Command{
		Use:   "collect",
		Short: "Collect a postmortem evidence bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				root = config.GetReportsPath()
			}

			now := time.Now().UTC()
			if ifDue {
				due, err := collectionDue(root, now)
				if err != nil {
					return err
				}
				if !due {
					fmt.Fprintf(cmd.OutOrStdout(), "postmortem skipped: latest successful bundle younger than %s\n", postmortemCadence)
					return nil
				}
			}

			db, err := database.Open()
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			logDir, activityDir, activityDirError := "", "", ""
			if configDir, err := paths.Plex2JellyfinDir(); err == nil {
				activityDir = filepath.Join(configDir, "activity")
			} else {
				activityDirError = fmt.Sprintf("resolve activity directory: %v", err)
			}
			if cfg, err := config.Load(); err == nil {
				if file, err := logging.ResolveFilePath(logging.Config{File: cfg.Logging.File}); err == nil {
					logDir = filepath.Dir(file)
				}
			}

			c := postmortem.Collector{
				DB:               db,
				Root:             root,
				Now:              func() time.Time { return now },
				Since:            now.Add(-since),
				LogDir:           logDir,
				ActivityDir:      activityDir,
				ActivityDirError: activityDirError,
				Workspace:        "/home/nomadx/Documents/plex2jellyfin",
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
	collect.Flags().BoolVar(&ifDue, "if-due", false, "skip when latest successful bundle is younger than 96h")
	cmd.AddCommand(collect)

	return cmd
}

// collectionDue reports whether a new bundle should be collected under root.
// Manual collection omits --if-due and never calls this.
func collectionDue(root string, now time.Time) (bool, error) {
	stamp, ok, err := latestSuccessfulBundleTime(root)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return !stamp.After(now.Add(-postmortemCadence)), nil
}

func latestSuccessfulBundleTime(root string) (time.Time, bool, error) {
	latest := filepath.Join(root, "latest")
	info, err := os.Lstat(latest)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}

	target := latest
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := os.Readlink(latest)
		if err != nil {
			return time.Time{}, false, err
		}
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(root, resolved)
		}
		target = resolved
	}

	summaryPath := filepath.Join(target, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}

	var summary struct {
		GeneratedAt time.Time `json:"generated_at"`
		RunID       string    `json:"run_id"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return time.Time{}, false, fmt.Errorf("parse %s: %w", summaryPath, err)
	}
	if !summary.GeneratedAt.IsZero() {
		return summary.GeneratedAt.UTC(), true, nil
	}
	if summary.RunID != "" {
		stamp, err := time.ParseInLocation(postmortem.TimestampLayout, summary.RunID, time.UTC)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("parse run_id %q: %w", summary.RunID, err)
		}
		return stamp, true, nil
	}
	return time.Time{}, false, nil
}
