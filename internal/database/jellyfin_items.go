package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// JellyfinItem tracks a media file path that has been confirmed by Jellyfin.
type JellyfinItem struct {
	ID             int64
	Path           string
	JellyfinItemID string
	ItemName       string
	ItemType       string
	ConfirmedAt    time.Time
	UpdatedAt      time.Time
}

// UpsertJellyfinItem stores Jellyfin confirmation data for a file path.
func (m *MediaDB) UpsertJellyfinItem(path, jellyfinItemID, itemName, itemType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path = strings.TrimSpace(path)
	jellyfinItemID = strings.TrimSpace(jellyfinItemID)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if jellyfinItemID == "" {
		return fmt.Errorf("jellyfin item id is required")
	}

	query := `
		INSERT INTO jellyfin_items (path, jellyfin_item_id, item_name, item_type, confirmed_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			jellyfin_item_id = excluded.jellyfin_item_id,
			item_name = excluded.item_name,
			item_type = excluded.item_type,
			updated_at = CURRENT_TIMESTAMP
	`

	if _, err := m.db.Exec(query, path, jellyfinItemID, itemName, itemType); err != nil {
		return fmt.Errorf("failed to upsert jellyfin item: %w", err)
	}
	return nil
}

// GetJellyfinItemByPath returns Jellyfin confirmation data for a given file path.
func (m *MediaDB) GetJellyfinItemByPath(path string) (*JellyfinItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	var item JellyfinItem
	query := `
		SELECT id, path, jellyfin_item_id, item_name, item_type, confirmed_at, updated_at
		FROM jellyfin_items
		WHERE path = ?
	`
	if err := m.db.QueryRow(query, path).Scan(
		&item.ID,
		&item.Path,
		&item.JellyfinItemID,
		&item.ItemName,
		&item.ItemType,
		&item.ConfirmedAt,
		&item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get jellyfin item by path: %w", err)
	}

	return &item, nil
}
