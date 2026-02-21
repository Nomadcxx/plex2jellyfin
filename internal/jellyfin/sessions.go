package jellyfin

import (
	"fmt"
	"path/filepath"
)

// GetSessions returns all active sessions.
func (c *Client) GetSessions() ([]Session, error) {
	var sessions []Session
	if err := c.get("/Sessions", &sessions); err != nil {
		return nil, fmt.Errorf("getting sessions: %w", err)
	}
	return sessions, nil
}

// IsPathBeingPlayed checks if any active session is streaming a file at the given path.
func (c *Client) IsPathBeingPlayed(filePath string) (bool, *Session, error) {
	sessions, err := c.GetSessions()
	if err != nil {
		return false, nil, fmt.Errorf("checking active sessions: %w", err)
	}

	normalizedPath := filepath.Clean(filePath)
	for i := range sessions {
		if sessions[i].NowPlayingItem == nil {
			continue
		}
		if filepath.Clean(sessions[i].NowPlayingItem.Path) == normalizedPath {
			return true, &sessions[i], nil
		}
	}

	return false, nil, nil
}

// GetActiveStreams returns sessions currently playing media.
func (c *Client) GetActiveStreams() ([]Session, error) {
	sessions, err := c.GetSessions()
	if err != nil {
		return nil, err
	}

	active := make([]Session, 0, len(sessions))
	for _, session := range sessions {
		if session.NowPlayingItem != nil {
			active = append(active, session)
		}
	}

	return active, nil
}
