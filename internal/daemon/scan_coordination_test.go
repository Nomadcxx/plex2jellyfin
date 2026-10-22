package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/require"
)

func TestMediaHandlerWiresPluginScanCoordination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Plex2Jellyfin/active-scans", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(jellyfin.ActiveScansResponse{
			Scans: []jellyfin.LibraryScan{{LibraryID: "movies"}},
		}))
	}))
	defer server.Close()

	library := t.TempDir()
	source := filepath.Join(t.TempDir(), "Example (2026).mkv")
	require.NoError(t, os.WriteFile(source, []byte("keep source"), 0o644))
	handler, err := NewMediaHandler(MediaHandlerConfig{
		MovieLibs:    []string{library},
		Timeout:      time.Nanosecond,
		Backend:      transfer.BackendNative,
		TargetUID:    -1,
		TargetGID:    -1,
		PluginClient: jellyfin.NewPluginClient(jellyfin.Config{URL: server.URL}),
	})
	require.NoError(t, err)
	defer handler.Shutdown()

	result, err := handler.movieOrganizer.OrganizeMovieWithParsed(source, library, naming.MovieInfo{Title: "Example", Year: "2026"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.ErrorContains(t, result.Error, "timeout waiting for Jellyfin scan to complete")
	require.False(t, result.Success)
	require.FileExists(t, source)
	require.NoFileExists(t, filepath.Join(library, "Example (2026)", "Example (2026).mkv"))
}
