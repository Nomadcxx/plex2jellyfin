package sonarr

import (
	"fmt"
	"strings"
)

func (c *Client) GetAllSeries() ([]Series, error) {
	var series []Series
	if err := c.get("/api/v3/series", &series); err != nil {
		return nil, fmt.Errorf("getting series: %w", err)
	}
	return series, nil
}

func (c *Client) GetSeries(id int) (*Series, error) {
	endpoint := fmt.Sprintf("/api/v3/series/%d", id)
	var series Series
	if err := c.get(endpoint, &series); err != nil {
		return nil, fmt.Errorf("getting series %d: %w", id, err)
	}
	return &series, nil
}

func (c *Client) GetSeriesByTvdbID(tvdbID int) (*Series, error) {
	endpoint := fmt.Sprintf("/api/v3/series?tvdbId=%d", tvdbID)
	var series []Series
	if err := c.get(endpoint, &series); err != nil {
		return nil, fmt.Errorf("getting series by tvdb %d: %w", tvdbID, err)
	}
	if len(series) == 0 {
		return nil, fmt.Errorf("no series found with tvdb ID %d", tvdbID)
	}
	return &series[0], nil
}

func (c *Client) FindSeriesByTitle(title string) ([]Series, error) {
	allSeries, err := c.GetAllSeries()
	if err != nil {
		return nil, err
	}

	titleLower := strings.ToLower(title)
	var matches []Series

	for _, s := range allSeries {
		if strings.Contains(strings.ToLower(s.Title), titleLower) ||
			strings.Contains(strings.ToLower(s.TitleSlug), titleLower) {
			matches = append(matches, s)
		}
	}

	return matches, nil
}

func (c *Client) GetEpisodes(seriesID int) ([]Episode, error) {
	endpoint := fmt.Sprintf("/api/v3/episode?seriesId=%d", seriesID)
	var episodes []Episode
	if err := c.get(endpoint, &episodes); err != nil {
		return nil, fmt.Errorf("getting episodes for series %d: %w", seriesID, err)
	}
	return episodes, nil
}

func (c *Client) GetEpisode(id int) (*Episode, error) {
	endpoint := fmt.Sprintf("/api/v3/episode/%d", id)
	var episode Episode
	if err := c.get(endpoint, &episode); err != nil {
		return nil, fmt.Errorf("getting episode %d: %w", id, err)
	}
	return &episode, nil
}

func (c *Client) GetEpisodeWithDetails(id int) (*Episode, error) {
	endpoint := fmt.Sprintf("/api/v3/episode/%d?includeSeries=true&includeEpisodeFile=true", id)
	var episode Episode
	if err := c.get(endpoint, &episode); err != nil {
		return nil, fmt.Errorf("getting episode %d with details: %w", id, err)
	}
	return &episode, nil
}

func (c *Client) GetEpisodeFile(id int) (*EpisodeFile, error) {
	endpoint := fmt.Sprintf("/api/v3/episodefile/%d", id)
	var file EpisodeFile
	if err := c.get(endpoint, &file); err != nil {
		return nil, fmt.Errorf("getting episode file %d: %w", id, err)
	}
	return &file, nil
}

func (c *Client) GetEpisodeFiles(seriesID int) ([]EpisodeFile, error) {
	endpoint := fmt.Sprintf("/api/v3/episodefile?seriesId=%d", seriesID)
	var files []EpisodeFile
	if err := c.get(endpoint, &files); err != nil {
		return nil, fmt.Errorf("getting episode files for series %d: %w", seriesID, err)
	}
	return files, nil
}

func (c *Client) FindEpisode(seriesID, seasonNumber, episodeNumber int) (*Episode, error) {
	episodes, err := c.GetEpisodes(seriesID)
	if err != nil {
		return nil, err
	}

	for _, ep := range episodes {
		if ep.SeasonNumber == seasonNumber && ep.EpisodeNumber == episodeNumber {
			return &ep, nil
		}
	}

	return nil, fmt.Errorf("episode S%02dE%02d not found for series %d",
		seasonNumber, episodeNumber, seriesID)
}

// UpdateSeriesPath updates the path for a series in Sonarr
// This is used when Plex2Jellyfin moves a series to a new location
func (c *Client) UpdateSeriesPath(seriesID int, newPath string) error {
	// First get the existing series
	series, err := c.GetSeries(seriesID)
	if err != nil {
		return fmt.Errorf("getting series for update: %w", err)
	}

	// Update the path
	series.Path = newPath

	// PUT the updated series back
	endpoint := fmt.Sprintf("/api/v3/series/%d", seriesID)
	var updated Series
	if err := c.put(endpoint, series, &updated); err != nil {
		return fmt.Errorf("updating series path: %w", err)
	}

	return nil
}
