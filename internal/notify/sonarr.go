package notify

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

type SonarrNotifier struct {
	client  *sonarr.Client
	db      *database.MediaDB
	enabled bool
}

func NewSonarrNotifier(client *sonarr.Client, db *database.MediaDB, enabled bool) *SonarrNotifier {
	return &SonarrNotifier{
		client:  client,
		db:      db,
		enabled: enabled && client != nil && db != nil,
	}
}

func (n *SonarrNotifier) Name() string {
	return "sonarr"
}

func (n *SonarrNotifier) Enabled() bool {
	return n.enabled
}

func (n *SonarrNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	return n.client.Ping()
}

func (n *SonarrNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{
		Service: n.Name(),
	}

	if event.MediaType != MediaTypeTVEpisode {
		result.Skipped = true
		result.Duration = time.Since(start)
		return result
	}

	year, _ := strconv.Atoi(event.Year)
	allSeries, err := n.client.GetAllSeries()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	series, err := matchSonarrSeries(allSeries, event.Title, year)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	record, err := n.db.GetSeriesByTitle(event.Title, year)
	if err != nil || record == nil {
		if err == nil {
			err = fmt.Errorf("Plex2Jellyfin has no canonical series record for %q (%d)", event.Title, year)
		}
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}
	targetDir := filepath.Clean(record.CanonicalPath)
	organizedSeriesDir := filepath.Clean(filepath.Dir(filepath.Dir(event.TargetPath)))
	if targetDir != organizedSeriesDir {
		result.Error = fmt.Errorf("organized path %q disagrees with Plex2Jellyfin canonical path %q", organizedSeriesDir, targetDir)
		result.Duration = time.Since(start)
		return result
	}

	sonarrID := series.ID
	record.SonarrID = &sonarrID
	if record.Year == 0 && series.Year > 0 {
		record.Year = series.Year
	}
	if series.TvdbID > 0 {
		tvdbID := series.TvdbID
		record.TvdbID = &tvdbID
	}
	if series.ImdbID != "" {
		imdbID := series.ImdbID
		record.ImdbID = &imdbID
	}
	if _, err := n.db.UpsertSeries(record); err != nil {
		result.Error = fmt.Errorf("recording Sonarr identity in Plex2Jellyfin: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if filepath.Clean(series.Path) != targetDir {
		if err := n.client.UpdateSeriesPath(series.ID, targetDir); err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result
		}
	}
	resp, err := n.client.RescanSeries(series.ID)
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

func matchSonarrSeries(allSeries []sonarr.Series, title string, year int) (*sonarr.Series, error) {
	normalized := database.NormalizeTitle(title)
	if normalized == "" {
		return nil, fmt.Errorf("cannot match a Sonarr series without a title")
	}
	var match *sonarr.Series
	for i := range allSeries {
		series := &allSeries[i]
		if database.NormalizeTitle(series.Title) != normalized {
			continue
		}
		if year > 0 && series.Year != year {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("multiple Sonarr series match %q (%d)", title, year)
		}
		match = series
	}
	if match == nil {
		return nil, fmt.Errorf("no Sonarr series matches %q (%d)", title, year)
	}
	return match, nil
}
