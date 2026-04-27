package jellyfin

import "testing"

func TestPathTranslator_JellyfinToDaemon(t *testing.T) {
	tr := NewPathTranslator([]PathMapping{
		{Jellyfin: "/tv5", Daemon: "/mnt/STORAGE5/TVSHOWS"},
		{Jellyfin: "/movies", Daemon: "/mnt/STORAGE2/MOVIES"},
	})
	cases := []struct {
		in, want string
	}{
		{"/tv5/Tracker (2024)/Season 03/Tracker (2024) S03E18.mkv", "/mnt/STORAGE5/TVSHOWS/Tracker (2024)/Season 03/Tracker (2024) S03E18.mkv"},
		{"/movies/Heat (1995)/Heat (1995).mkv", "/mnt/STORAGE2/MOVIES/Heat (1995)/Heat (1995).mkv"},
		{"/tv5", "/mnt/STORAGE5/TVSHOWS"},
		{"/unmapped/path/file.mkv", "/unmapped/path/file.mkv"},
		{"", ""},
	}
	for _, c := range cases {
		got := tr.JellyfinToDaemon(c.in)
		if got != c.want {
			t.Errorf("JellyfinToDaemon(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPathTranslator_DaemonToJellyfin(t *testing.T) {
	tr := NewPathTranslator([]PathMapping{
		{Jellyfin: "/tv5", Daemon: "/mnt/STORAGE5/TVSHOWS"},
	})
	got := tr.DaemonToJellyfin("/mnt/STORAGE5/TVSHOWS/Foo/Foo S01E01.mkv")
	want := "/tv5/Foo/Foo S01E01.mkv"
	if got != want {
		t.Errorf("DaemonToJellyfin = %q, want %q", got, want)
	}
}

func TestPathTranslator_LongestPrefixFirst(t *testing.T) {
	tr := NewPathTranslator([]PathMapping{
		{Jellyfin: "/tv", Daemon: "/mnt/short"},
		{Jellyfin: "/tv5", Daemon: "/mnt/STORAGE5/TVSHOWS"},
	})
	got := tr.JellyfinToDaemon("/tv5/Foo/file.mkv")
	want := "/mnt/STORAGE5/TVSHOWS/Foo/file.mkv"
	if got != want {
		t.Errorf("longest-prefix wins: got %q, want %q", got, want)
	}
}

func TestPathTranslator_BoundaryGuard(t *testing.T) {
	tr := NewPathTranslator([]PathMapping{
		{Jellyfin: "/mnt/STORAGE5", Daemon: "/d5"},
	})
	// "/mnt/STORAGE50" must not be translated.
	got := tr.JellyfinToDaemon("/mnt/STORAGE50/file.mkv")
	if got != "/mnt/STORAGE50/file.mkv" {
		t.Errorf("boundary guard failed: %q", got)
	}
}

func TestPathTranslator_NilSafe(t *testing.T) {
	var tr *PathTranslator
	if got := tr.JellyfinToDaemon("/x"); got != "/x" {
		t.Errorf("nil translator should be a no-op: %q", got)
	}
	if got := tr.DaemonToJellyfin("/x"); got != "/x" {
		t.Errorf("nil translator should be a no-op: %q", got)
	}
}

func TestPathTranslator_EmptyMappingsSkipped(t *testing.T) {
	tr := NewPathTranslator([]PathMapping{
		{Jellyfin: "", Daemon: "/d"},
		{Jellyfin: "/j", Daemon: ""},
		{Jellyfin: "/tv5", Daemon: "/mnt/STORAGE5"},
	})
	if got := tr.JellyfinToDaemon("/tv5/x.mkv"); got != "/mnt/STORAGE5/x.mkv" {
		t.Errorf("got %q", got)
	}
}
