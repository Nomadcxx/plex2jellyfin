package daemon

import "testing"

func TestNormalizeEventPath(t *testing.T) {
	tests := []struct {
		name       string
		rawPath    string
		watchRoots []string
		wantReason string
		wantAction ingestAction
	}{
		{
			name:       "empty path",
			rawPath:    "",
			watchRoots: []string{"/watch"},
			wantReason: "empty_path",
			wantAction: ingestReject,
		},
		{
			name:       "relative path",
			rawPath:    "movie.mkv",
			watchRoots: []string{"/watch"},
			wantReason: "not_absolute",
			wantAction: ingestReject,
		},
		{
			name:       "truncated root path",
			rawPath:    "/movie.mkv",
			watchRoots: []string{"/watch"},
			wantReason: "truncated_root_path",
			wantAction: ingestReject,
		},
		{
			name:       "outside roots",
			rawPath:    "/other/movie.mkv",
			watchRoots: []string{"/watch"},
			wantReason: "outside_watch_roots",
			wantAction: ingestReject,
		},
		{
			name:       "transient unpack path",
			rawPath:    "/watch/tv/_UNPACK_abc/show.mkv",
			watchRoots: []string{"/watch/tv"},
			wantReason: "transient_unpack_path",
			wantAction: ingestDefer,
		},
		{
			name:       "valid path under root",
			rawPath:    "/watch/movies/movie.mkv",
			watchRoots: []string{"/watch"},
			wantReason: "",
			wantAction: ingestAccept,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason, action := normalizeEventPath(tt.rawPath, tt.watchRoots)
			if reason != tt.wantReason || action != tt.wantAction {
				t.Fatalf("got reason=%q action=%v", reason, action)
			}
		})
	}
}
