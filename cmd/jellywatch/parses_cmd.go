package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/paths"
	"github.com/spf13/cobra"
)

// validHumanLabels is the set of accepted values for human_label_override.
var validHumanLabels = map[string]bool{
	"ok":    true,
	"wrong": true,
	"drift": true,
	"fail":  true,
}

func newParsesCmd() *cobra.Command {
	return newParsesCmdWithDeps(
		func() (*database.MediaDB, error) {
			dbPath, err := paths.DatabasePath()
			if err != nil {
				return nil, err
			}
			return database.OpenPath(dbPath)
		},
		os.Stdout,
		os.Stderr,
	)
}

func newParsesCmdWithDeps(openDB func() (*database.MediaDB, error), stdout, stderr io.Writer) *cobra.Command {
	var (
		failures       bool
		drift          bool
		sourceContains string
		overrideID     int64
		label          string
		limit          int
	)

	cmd := &cobra.Command{
		Use:   "parses",
		Short: "Query and manage parse decisions",
		Long:  "Query parse decision records and update human label overrides.",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			// Handle override update mode.
			if cmd.Flags().Changed("override") {
				if label == "" {
					return fmt.Errorf("--label is required with --override")
				}
				if !validHumanLabels[strings.ToLower(label)] {
					return fmt.Errorf("invalid label %q: must be one of ok, wrong, drift, fail", label)
				}
				if err := db.UpdateHumanOverride(overrideID, label); err != nil {
					return fmt.Errorf("update override: %w", err)
				}
				fmt.Fprintf(stdout, "updated human_label_override for id %d to %q\n", overrideID, label)
				return nil
			}

			// Query mode.
			filter := database.QueryFilter{
				SourceContains: sourceContains,
				Limit:          limit,
			}
			if failures {
				filter.AutoLabel = "FAIL"
			}
			if drift {
				filter.AutoLabel = "DRIFT"
			}

			rows, err := db.QueryDecisions(filter)
			if err != nil {
				return fmt.Errorf("query decisions: %w", err)
			}

			if len(rows) == 0 {
				fmt.Fprintln(stdout, "no parse decisions found")
				return nil
			}

			for _, d := range rows {
				yearStr := ""
				if d.ParsedYear != nil {
					yearStr = strconv.Itoa(*d.ParsedYear)
				}
				seasonStr := ""
				if d.ParsedSeason != nil {
					seasonStr = fmt.Sprintf("S%02d", *d.ParsedSeason)
				}
				epStr := ""
				if d.ParsedEpisode != nil {
					epStr = fmt.Sprintf("E%02d", *d.ParsedEpisode)
				}
				fmt.Fprintf(stdout, "id=%-6d source=%-60s title=%q year=%s %s%s auto_label=%s human_override=%s\n",
					d.ID,
					d.SourceFilename,
					d.ParsedTitle,
					yearStr,
					seasonStr,
					epStr,
					d.AutoLabel,
					d.HumanLabelOverride,
				)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&failures, "failures", false, "show only rows with auto_label=FAIL")
	cmd.Flags().BoolVar(&drift, "drift", false, "show only rows with auto_label=DRIFT")
	cmd.Flags().StringVar(&sourceContains, "source-contains", "", "filter by source path substring")
	cmd.Flags().Int64Var(&overrideID, "override", 0, "row ID to set human_label_override on")
	cmd.Flags().StringVar(&label, "label", "", "human label value (ok|wrong|drift|fail); required with --override")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum rows to return")

	return cmd
}
