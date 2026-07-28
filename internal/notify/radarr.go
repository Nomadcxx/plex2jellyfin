package notify

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/radarr"
	"github.com/Nomadcxx/plex2jellyfin/internal/service"
)

type RadarrNotifier struct {
	client  *radarr.Client
	db      *database.MediaDB
	enabled bool
}

func NewRadarrNotifier(client *radarr.Client, db *database.MediaDB, enabled bool) *RadarrNotifier {
	return &RadarrNotifier{
		client:  client,
		db:      db,
		enabled: enabled && client != nil && db != nil,
	}
}

func (n *RadarrNotifier) Name() string {
	return "radarr"
}

func (n *RadarrNotifier) Enabled() bool {
	return n.enabled
}

func (n *RadarrNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	return n.client.Ping()
}

func (n *RadarrNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{
		Service: n.Name(),
	}

	if event.MediaType != MediaTypeMovie {
		result.Skipped = true
		result.Duration = time.Since(start)
		return result
	}
	issues, err := service.CheckRadarrConfig(n.client)
	if err != nil {
		result.Error = fmt.Errorf("checking Radarr compatibility: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	if len(issues) > 0 {
		result.Error = fmt.Errorf("Radarr is incompatible: %s=%s", issues[0].Setting, issues[0].Current)
		result.Duration = time.Since(start)
		return result
	}

	year, _ := strconv.Atoi(event.Year)
	movies, err := n.client.GetMovies()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	movie, err := matchRadarrMovie(movies, event.Title, year)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	record, err := n.db.GetMovieByTitle(event.Title, year)
	if err != nil || record == nil {
		if err == nil {
			err = fmt.Errorf("Plex2Jellyfin has no canonical movie record for %q (%d)", event.Title, year)
		}
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}
	targetDir := filepath.Clean(record.CanonicalPath)
	if targetDir != filepath.Clean(filepath.Dir(event.TargetPath)) {
		result.Error = fmt.Errorf("organized path %q disagrees with Plex2Jellyfin canonical path %q", filepath.Dir(event.TargetPath), targetDir)
		result.Duration = time.Since(start)
		return result
	}

	radarrID := movie.ID
	record.RadarrID = &radarrID
	if record.Year == 0 && movie.Year > 0 {
		record.Year = movie.Year
	}
	if movie.TmdbID > 0 {
		tmdbID := movie.TmdbID
		record.TmdbID = &tmdbID
	}
	if movie.ImdbID != "" {
		imdbID := movie.ImdbID
		record.ImdbID = &imdbID
	}
	if _, err := n.db.UpsertMovie(record); err != nil {
		result.Error = fmt.Errorf("recording Radarr identity in Plex2Jellyfin: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if filepath.Clean(movie.Path) != targetDir {
		if err := n.client.UpdateMoviePath(movie.ID, targetDir); err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result
		}
	}
	resp, err := n.client.RescanMovie(movie.ID)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	result.Success = true
	result.CommandID = resp.ID
	result.Duration = time.Since(start)
	return result
}

func matchRadarrMovie(movies []radarr.Movie, title string, year int) (*radarr.Movie, error) {
	normalized := database.NormalizeTitle(title)
	if normalized == "" {
		return nil, fmt.Errorf("cannot match a Radarr movie without a title")
	}
	var match *radarr.Movie
	for i := range movies {
		movie := &movies[i]
		if database.NormalizeTitle(movie.Title) != normalized &&
			database.NormalizeTitle(movie.OriginalTitle) != normalized {
			continue
		}
		if year > 0 && movie.Year != year {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple Radarr movies match %q (%d)", title, year)
		}
		match = movie
	}
	if match == nil {
		return nil, fmt.Errorf("no Radarr movie matches %q (%d)", title, year)
	}
	return match, nil
}
