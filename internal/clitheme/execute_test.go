package clitheme

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestInjectRootBanner_PrependsOnce(t *testing.T) {
	root := &cobra.Command{
		Use:  "plex2jellyfin",
		Long: "Plex2Jellyfin helps review libraries.",
	}
	injectRootBanner(root, "dev")
	if !strings.Contains(root.Long, "▄▄▄▄▄") {
		t.Fatalf("banner missing from Long:\n%s", root.Long)
	}
	if !strings.Contains(root.Long, "Plex2Jellyfin helps") {
		t.Fatalf("original Long lost:\n%s", root.Long)
	}
	once := root.Long
	injectRootBanner(root, "dev")
	if root.Long != once {
		t.Fatal("injectRootBanner should be idempotent")
	}
}
