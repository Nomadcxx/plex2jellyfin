package sonarr

import (
	"fmt"
	"time"
)

func (c *Client) ExecuteCommand(cmd Command) (*CommandResponse, error) {
	var response CommandResponse
	if err := c.post("/api/v3/command", cmd, &response); err != nil {
		return nil, fmt.Errorf("executing command %s: %w", cmd.Name, err)
	}
	return &response, nil
}

func (c *Client) TriggerDownloadedEpisodesScan(path string) (*CommandResponse, error) {
	cmd := Command{
		Name:       "DownloadedEpisodesScan",
		Path:       path,
		ImportMode: "Move",
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) TriggerDownloadedEpisodesScanCopy(path string) (*CommandResponse, error) {
	cmd := Command{
		Name:       "DownloadedEpisodesScan",
		Path:       path,
		ImportMode: "Copy",
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RefreshSeries(seriesID int) (*CommandResponse, error) {
	cmd := Command{
		Name:     "RefreshSeries",
		SeriesID: seriesID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RefreshAllSeries() (*CommandResponse, error) {
	cmd := Command{
		Name: "RefreshSeries",
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RescanSeries(seriesID int) (*CommandResponse, error) {
	cmd := Command{
		Name:     "RescanSeries",
		SeriesID: seriesID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RenameSeries(seriesID int) (*CommandResponse, error) {
	cmd := Command{
		Name:      "RenameSeries",
		SeriesIDs: []int{seriesID},
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RenameFiles(fileIDs []int) (*CommandResponse, error) {
	cmd := Command{
		Name:  "RenameFiles",
		Files: fileIDs,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) EpisodeSearch(episodeIDs []int) (*CommandResponse, error) {
	cmd := Command{
		Name:       "EpisodeSearch",
		EpisodeIDs: episodeIDs,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) SeasonSearch(seriesID, seasonNumber int) (*CommandResponse, error) {
	cmd := Command{
		Name:         "SeasonSearch",
		SeasonNumber: seasonNumber,
		SeriesID:     seriesID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) SeriesSearch(seriesID int) (*CommandResponse, error) {
	cmd := Command{
		Name:     "SeriesSearch",
		SeriesID: seriesID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RssSync() (*CommandResponse, error) {
	cmd := Command{
		Name: "RssSync",
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) GetCommandStatus(commandID int) (*CommandResponse, error) {
	endpoint := fmt.Sprintf("/api/v3/command/%d", commandID)
	var response CommandResponse
	if err := c.get(endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting command status %d: %w", commandID, err)
	}
	return &response, nil
}

func (c *Client) WaitForCommand(commandID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("command %d timed out after %s", commandID, timeout)
			}

			status, err := c.GetCommandStatus(commandID)
			if err != nil {
				return fmt.Errorf("checking command status: %w", err)
			}

			switch status.Status {
			case "completed":
				return nil
			case "failed":
				return fmt.Errorf("command failed: %s", status.Message)
			case "aborted":
				return fmt.Errorf("command was aborted")
			}
		}
	}
}

func (c *Client) GetHistory(page, pageSize int) (*HistoryResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	endpoint := fmt.Sprintf("/api/v3/history?page=%d&pageSize=%d&includeSeries=true&includeEpisode=true",
		page, pageSize)
	var response HistoryResponse
	if err := c.get(endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting history: %w", err)
	}
	return &response, nil
}

func (c *Client) GetHistoryForEpisode(episodeID int) ([]HistoryRecord, error) {
	endpoint := fmt.Sprintf("/api/v3/history?episodeId=%d", episodeID)
	var response HistoryResponse
	if err := c.get(endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting history for episode %d: %w", episodeID, err)
	}
	return response.Records, nil
}

func (c *Client) GetHistoryByDownloadID(downloadID string) ([]HistoryRecord, error) {
	endpoint := fmt.Sprintf("/api/v3/history?downloadId=%s", downloadID)
	var response HistoryResponse
	if err := c.get(endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting history for download %s: %w", downloadID, err)
	}
	return response.Records, nil
}
