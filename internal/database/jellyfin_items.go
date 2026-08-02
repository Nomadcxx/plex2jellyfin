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

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin jellyfin item upsert: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO jellyfin_items (path, jellyfin_item_id, item_name, item_type, confirmed_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			jellyfin_item_id = excluded.jellyfin_item_id,
			item_name = excluded.item_name,
			item_type = excluded.item_type,
			updated_at = CURRENT_TIMESTAMP
	`
	if _, err := tx.Exec(query, path, jellyfinItemID, itemName, itemType); err != nil {
		return fmt.Errorf("failed to upsert jellyfin item: %w", err)
	}
	if err := bumpJellyfinInventoryIncompleteTx(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit jellyfin item upsert: %w", err)
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

// DeleteJellyfinItemByPath removes cached Jellyfin confirmation data.
// Deleting a path that is not cached is a successful no-op.
func (m *MediaDB) DeleteJellyfinItemByPath(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin jellyfin item delete: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM jellyfin_items WHERE path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete jellyfin item by path: %w", err)
	}
	if err := bumpJellyfinInventoryIncompleteTx(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit jellyfin item delete: %w", err)
	}
	return nil
}

// ListJellyfinItems returns all cached Jellyfin confirmations ordered by path.
func (m *MediaDB) ListJellyfinItems() ([]JellyfinItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT id, path, jellyfin_item_id, item_name, item_type, confirmed_at, updated_at
		FROM jellyfin_items
		ORDER BY path
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list jellyfin items: %w", err)
	}
	defer rows.Close()

	var items []JellyfinItem
	for rows.Next() {
		var item JellyfinItem
		if err := rows.Scan(
			&item.ID,
			&item.Path,
			&item.JellyfinItemID,
			&item.ItemName,
			&item.ItemType,
			&item.ConfirmedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan jellyfin item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list jellyfin items: %w", err)
	}
	return items, nil
}

// CountJellyfinItems returns the number of cached Jellyfin confirmations.
func (m *MediaDB) CountJellyfinItems() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM jellyfin_items`).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count jellyfin items: %w", err)
	}
	return count, nil
}

// ReconcileJellyfinItems atomically replaces the cache with a complete
// Jellyfin inventory and returns paths removed from the prior snapshot.
func (m *MediaDB) ReconcileJellyfinItems(items []JellyfinItem) ([]string, error) {
	live := make(map[string]JellyfinItem, len(items))
	for _, item := range items {
		item.Path = strings.TrimSpace(item.Path)
		item.JellyfinItemID = strings.TrimSpace(item.JellyfinItemID)
		if item.Path == "" {
			return nil, fmt.Errorf("path is required")
		}
		if item.JellyfinItemID == "" {
			return nil, fmt.Errorf("jellyfin item id is required")
		}
		live[item.Path] = item
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin jellyfin item reconciliation: %w", err)
	}
	defer tx.Rollback()

	for _, item := range live {
		if _, err := tx.Exec(`
			INSERT INTO jellyfin_items (path, jellyfin_item_id, item_name, item_type, confirmed_at, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT(path) DO UPDATE SET
				jellyfin_item_id = excluded.jellyfin_item_id,
				item_name = excluded.item_name,
				item_type = excluded.item_type,
				updated_at = CURRENT_TIMESTAMP
		`, item.Path, item.JellyfinItemID, item.ItemName, item.ItemType); err != nil {
			return nil, fmt.Errorf("failed to reconcile jellyfin item: %w", err)
		}
	}

	rows, err := tx.Query(`SELECT path FROM jellyfin_items ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("failed to list reconciled jellyfin items: %w", err)
	}
	var removed []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan reconciled jellyfin item: %w", err)
		}
		if _, ok := live[path]; !ok {
			removed = append(removed, path)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("failed to list reconciled jellyfin items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("failed to close reconciled jellyfin items: %w", err)
	}

	for _, path := range removed {
		if _, err := tx.Exec(`DELETE FROM jellyfin_items WHERE path = ?`, path); err != nil {
			return nil, fmt.Errorf("failed to remove stale jellyfin item: %w", err)
		}
	}
	nextGen, err := nextJellyfinInventoryGenerationTx(tx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`
		INSERT INTO jellyfin_inventory_state (singleton, generation, complete, completed_at, item_count)
		VALUES (1, ?, 1, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			generation = excluded.generation,
			complete = 1,
			completed_at = excluded.completed_at,
			item_count = excluded.item_count
	`, nextGen, len(live)); err != nil {
		return nil, fmt.Errorf("failed to mark jellyfin inventory complete: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit jellyfin item reconciliation: %w", err)
	}
	return removed, nil
}

// JellyfinInventoryGeneration returns the current inventory generation and
// whether that generation is an authoritative complete snapshot.
func (m *MediaDB) JellyfinInventoryGeneration() (int64, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var generation int64
	var complete int
	var expected, actual int
	err := m.db.QueryRow(`
		SELECT generation, complete, item_count, (SELECT COUNT(*) FROM jellyfin_items)
		FROM jellyfin_inventory_state
		WHERE singleton = 1
	`).Scan(&generation, &complete, &expected, &actual)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to read jellyfin inventory generation: %w", err)
	}
	return generation, complete == 1 && expected == actual, nil
}

// IsJellyfinInventoryComplete reports whether the cache exactly matches the
// last completed reconciliation and has not been changed by webhook updates.
func (m *MediaDB) IsJellyfinInventoryComplete() (bool, error) {
	_, complete, err := m.JellyfinInventoryGeneration()
	return complete, err
}

// MarkJellyfinInventoryIncomplete prevents verification from trusting the
// cache while an inventory fetch is in progress or after it fails.
func (m *MediaDB) MarkJellyfinInventoryIncomplete() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin jellyfin inventory invalidation: %w", err)
	}
	defer tx.Rollback()

	if err := bumpJellyfinInventoryIncompleteTx(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit jellyfin inventory invalidation: %w", err)
	}
	return nil
}

func nextJellyfinInventoryGenerationTx(tx *sql.Tx) (int64, error) {
	var current sql.NullInt64
	if err := tx.QueryRow(`
		SELECT generation FROM jellyfin_inventory_state WHERE singleton = 1
	`).Scan(&current); err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to read jellyfin inventory generation: %w", err)
	}
	if current.Valid {
		return current.Int64 + 1, nil
	}
	return 1, nil
}

func bumpJellyfinInventoryIncompleteTx(tx *sql.Tx) error {
	nextGen, err := nextJellyfinInventoryGenerationTx(tx)
	if err != nil {
		return err
	}
	var itemCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM jellyfin_items`).Scan(&itemCount); err != nil {
		return fmt.Errorf("failed to count jellyfin items: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO jellyfin_inventory_state (singleton, generation, complete, completed_at, item_count)
		VALUES (1, ?, 0, NULL, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			generation = excluded.generation,
			complete = 0,
			completed_at = NULL,
			item_count = excluded.item_count
	`, nextGen, itemCount); err != nil {
		return fmt.Errorf("failed to mark jellyfin inventory incomplete: %w", err)
	}
	return nil
}
