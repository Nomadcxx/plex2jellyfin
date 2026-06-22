package radarr

import (
	"context"
	"fmt"
	"time"
)

func (c *Client) ExecuteCommand(cmd Command) (*CommandResponse, error) {
	return c.ExecuteCommandContext(context.Background(), cmd)
}

func (c *Client) ExecuteCommandContext(ctx context.Context, cmd Command) (*CommandResponse, error) {
	var response CommandResponse
	if err := c.postContext(ctx, "/api/v3/command", cmd, &response); err != nil {
		return nil, fmt.Errorf("executing command %s: %w", cmd.Name, err)
	}
	return &response, nil
}

func (c *Client) TriggerDownloadedMoviesScan(path string) (*CommandResponse, error) {
	return c.TriggerDownloadedMoviesScanContext(context.Background(), path)
}

func (c *Client) TriggerDownloadedMoviesScanContext(ctx context.Context, path string) (*CommandResponse, error) {
	cmd := Command{
		Name:       "DownloadedMoviesScan",
		Path:       path,
		ImportMode: "Move",
	}
	return c.ExecuteCommandContext(ctx, cmd)
}

func (c *Client) TriggerDownloadedMoviesScanCopy(path string) (*CommandResponse, error) {
	return c.TriggerDownloadedMoviesScanCopyContext(context.Background(), path)
}

func (c *Client) TriggerDownloadedMoviesScanCopyContext(ctx context.Context, path string) (*CommandResponse, error) {
	cmd := Command{
		Name:       "DownloadedMoviesScan",
		Path:       path,
		ImportMode: "Copy",
	}
	return c.ExecuteCommandContext(ctx, cmd)
}

func (c *Client) RefreshMovie(movieID int) (*CommandResponse, error) {
	cmd := Command{
		Name:    "RefreshMovie",
		MovieID: movieID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RefreshAllMovies() (*CommandResponse, error) {
	return c.RefreshAllMoviesContext(context.Background())
}

func (c *Client) RefreshAllMoviesContext(ctx context.Context) (*CommandResponse, error) {
	cmd := Command{
		Name: "RefreshMovie",
	}
	return c.ExecuteCommandContext(ctx, cmd)
}

func (c *Client) RescanMovie(movieID int) (*CommandResponse, error) {
	cmd := Command{
		Name:    "RescanMovie",
		MovieID: movieID,
	}
	return c.ExecuteCommand(cmd)
}

func (c *Client) RenameMovie(movieID int) (*CommandResponse, error) {
	cmd := Command{
		Name:     "RenameMovie",
		MovieIDs: []int{movieID},
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

func (c *Client) MovieSearch(movieID int) (*CommandResponse, error) {
	cmd := Command{
		Name:     "MoviesSearch",
		MovieIDs: []int{movieID},
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
	return c.GetCommandStatusContext(context.Background(), commandID)
}

func (c *Client) GetCommandStatusContext(ctx context.Context, commandID int) (*CommandResponse, error) {
	endpoint := fmt.Sprintf("/api/v3/command/%d", commandID)
	var response CommandResponse
	if err := c.getContext(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting command status %d: %w", commandID, err)
	}
	return &response, nil
}

func (c *Client) WaitForCommand(commandID int, timeout time.Duration) error {
	return c.WaitForCommandContext(context.Background(), commandID, timeout)
}

func (c *Client) WaitForCommandContext(ctx context.Context, commandID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("command %d timed out after %s", commandID, timeout)
			}

			status, err := c.GetCommandStatusContext(ctx, commandID)
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
