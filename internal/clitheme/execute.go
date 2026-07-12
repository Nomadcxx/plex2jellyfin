package clitheme

import (
	"context"
	"strings"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
)

// Execute runs root under Fang with the Plex-yellow theme and a yellow ASCII
// banner on root help. Completions stay disabled to match prior CLI behavior.
// Version stays on the dedicated `version` subcommand so `-v` remains verbose.
func Execute(ctx context.Context, root *cobra.Command, version string) error {
	injectRootBanner(root, version)

	return fang.Execute(ctx, root,
		fang.WithColorSchemeFunc(FangScheme),
		fang.WithoutCompletions(),
		fang.WithoutVersion(),
	)
}

// injectRootBanner prepends a yellow ASCII banner to the root Long help text.
// Fang re-styles Long via lipgloss; nested plex-yellow ANSI in the banner is preserved.
func injectRootBanner(root *cobra.Command, version string) {
	banner := BannerString(version)
	long := strings.TrimSpace(root.Long)
	if long == "" {
		root.Long = banner
		return
	}
	if strings.Contains(root.Long, "▄▄▄▄▄") {
		return
	}
	root.Long = banner + "\n" + long
}
