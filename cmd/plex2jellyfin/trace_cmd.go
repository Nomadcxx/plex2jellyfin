package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/spf13/cobra"
)

func newTraceCmd() *cobra.Command {
	return newTraceCmdWithDeps(
		func() (*database.MediaDB, error) {
			dbPath, err := paths.DatabasePath()
			if err != nil {
				return nil, err
			}
			return database.OpenPath(dbPath)
		},
		os.Stdout,
	)
}

func newTraceCmdWithDeps(openDB func() (*database.MediaDB, error), stdout io.Writer) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "trace [filename or path fragment]",
		Short: "Show what the daemon did to a file, step by step",
		Long: `Show each file's journey through the pipeline in plain language:
when it was detected, how it was parsed, where it was moved, and whether
Jellyfin confirmed it. Newest first. With no argument, shows the most
recent files; pass a filename fragment to filter.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer db.Close()

			query := ""
			if len(args) == 1 {
				query = args[0]
			}

			rows, err := db.QueryDecisions(database.QueryFilter{
				SourceContains: query,
				Limit:          limit,
			})
			if err != nil {
				return fmt.Errorf("query decisions: %w", err)
			}

			if len(rows) == 0 {
				if query == "" {
					fmt.Fprintln(stdout, "No pipeline activity recorded yet.")
				} else {
					fmt.Fprintf(stdout, "No pipeline activity found matching %q.\n", query)
				}
				return nil
			}

			for i, d := range rows {
				if i > 0 {
					fmt.Fprintln(stdout)
				}
				writeTrace(stdout, d)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "maximum files to show (newest first)")
	return cmd
}

const timeLayout = "2006-01-02 15:04:05"

// writeTrace renders one parse decision as a human-readable journey.
func writeTrace(w io.Writer, d *database.ParseDecision) {
	fmt.Fprintln(w, d.SourceFilename)

	dir := strings.TrimSuffix(strings.TrimSuffix(d.SourcePath, d.SourceFilename), "/")
	if dir == "" {
		dir = d.SourcePath
	}
	fmt.Fprintf(w, "  detected   %s  in %s\n", d.EventAt.Local().Format(timeLayout), dir)

	parsed := fmt.Sprintf("via %s", orUnknown(d.ParseMethod))
	if d.ParsedTitle != "" {
		parsed += fmt.Sprintf(" -> %q", d.ParsedTitle)
		if d.ParsedYear != nil {
			parsed += fmt.Sprintf(" (%d)", *d.ParsedYear)
		}
		if d.ParsedSeason != nil && d.ParsedEpisode != nil {
			parsed += fmt.Sprintf(" S%02dE%02d", *d.ParsedSeason, *d.ParsedEpisode)
		}
	}
	if d.ParserStrippedTokens != "" {
		parsed += fmt.Sprintf("  (stripped: %s)", d.ParserStrippedTokens)
	}
	fmt.Fprintf(w, "  parsed     %s\n", parsed)

	switch {
	case d.TargetPath != "" && d.TargetAt != nil:
		fmt.Fprintf(w, "  moved      -> %s  at %s\n", d.TargetPath, d.TargetAt.Local().Format(timeLayout))
	case d.TargetPath != "":
		fmt.Fprintf(w, "  moved      -> %s\n", d.TargetPath)
	default:
		fmt.Fprintln(w, "  moved      (not moved)")
	}

	switch {
	case d.JellyfinItemID != "":
		line := fmt.Sprintf("matched item %s", d.JellyfinItemID)
		var ids []string
		if d.JellyfinImdbID != "" {
			ids = append(ids, "imdb "+d.JellyfinImdbID)
		}
		if d.JellyfinTmdbID != "" {
			ids = append(ids, "tmdb "+d.JellyfinTmdbID)
		}
		if d.JellyfinTvdbID != "" {
			ids = append(ids, "tvdb "+d.JellyfinTvdbID)
		}
		if len(ids) > 0 {
			line += " (" + strings.Join(ids, ", ") + ")"
		}
		if d.JellyfinResolvedAt != nil {
			line += "  at " + d.JellyfinResolvedAt.Local().Format(timeLayout)
		}
		fmt.Fprintf(w, "  jellyfin   %s\n", line)
	case d.JellyfinIdentified != nil && !*d.JellyfinIdentified:
		fmt.Fprintln(w, "  jellyfin   seen but not identified yet")
	default:
		fmt.Fprintln(w, "  jellyfin   not confirmed yet")
	}

	outcome := orUnknown(d.OrganizeOutcome)
	if label := d.HumanLabelOverride; label != "" {
		outcome += "  [reviewed: " + label + "]"
	} else if d.AutoLabel != "" {
		outcome += "  [" + d.AutoLabel + "]"
	}
	fmt.Fprintf(w, "  outcome    %s\n", outcome)
	if d.OrganizeError != "" {
		fmt.Fprintf(w, "  error      %s\n", d.OrganizeError)
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
