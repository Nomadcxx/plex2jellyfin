package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration26NormalizesOnlyGUIDItemIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.SQL().Exec(`DELETE FROM schema_version WHERE version = 26`)
	require.NoError(t, err)

	dashedID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	uppercaseID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	nonGUIDID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	malformedID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	_, err = db.SQL().Exec(`UPDATE parse_decisions SET jellyfin_item_id = ? WHERE id = ?`,
		"4bceeac3-77fe-a6fa-39e9-7850c6b55e35", dashedID)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`UPDATE parse_decisions SET jellyfin_item_id = ? WHERE id = ?`,
		"4BCEEAC377FEA6FA39E97850C6B55E35", uppercaseID)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`UPDATE parse_decisions SET jellyfin_item_id = ? WHERE id = ?`,
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", nonGUIDID)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`UPDATE parse_decisions SET jellyfin_item_id = ? WHERE id = ?`,
		"4bceeac3-77fe-a6fa-39e9-7850c6b55e3-", malformedID)
	require.NoError(t, err)
	_, err = db.SQL().Exec(`INSERT INTO jellyfin_items (path, jellyfin_item_id) VALUES (?, ?)`,
		"/library/show.mkv", "7139ed15-7800-ee8d-2209-72268dd4cc5b")
	require.NoError(t, err)
	_, err = db.SQL().Exec(`INSERT INTO jellyfin_items (path, jellyfin_item_id) VALUES (?, ?)`,
		"/library/malformed.mkv", "7139ed15-7800-ee8d-2209-72268dd4cc5-")
	require.NoError(t, err)

	require.NoError(t, db.migrate())
	require.NoError(t, db.migrate(), "migration must be idempotent")

	readDecisionID := func(id int64) string {
		t.Helper()
		var got string
		require.NoError(t, db.SQL().QueryRow(
			`SELECT jellyfin_item_id FROM parse_decisions WHERE id = ?`, id).Scan(&got))
		return got
	}
	require.Equal(t, "4bceeac377fea6fa39e97850c6b55e35", readDecisionID(dashedID))
	require.Equal(t, "4bceeac377fea6fa39e97850c6b55e35", readDecisionID(uppercaseID))
	require.Equal(t, "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", readDecisionID(nonGUIDID))
	require.Equal(t, "4bceeac3-77fe-a6fa-39e9-7850c6b55e3-", readDecisionID(malformedID))

	var cachedID string
	require.NoError(t, db.SQL().QueryRow(
		`SELECT jellyfin_item_id FROM jellyfin_items WHERE path = ?`, "/library/show.mkv").Scan(&cachedID))
	require.Equal(t, "7139ed157800ee8d220972268dd4cc5b", cachedID)
	require.NoError(t, db.SQL().QueryRow(
		`SELECT jellyfin_item_id FROM jellyfin_items WHERE path = ?`, "/library/malformed.mkv").Scan(&cachedID))
	require.Equal(t, "7139ed15-7800-ee8d-2209-72268dd4cc5-", cachedID)
}

func TestMigration26ClearsOnlyLegacyMissingProviderContradictions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.SQL().Exec(`DELETE FROM schema_version WHERE version = 26`)
	require.NoError(t, err)

	contradictory, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	_, err = db.SQL().Exec(`
		UPDATE parse_decisions
		   SET jellyfin_identified = 1,
		       metadata_state = 'missing_provider_ids',
		       metadata_error = 'legacy',
		       next_metadata_check_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, contradictory)
	require.NoError(t, err)

	unresolved, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	_, err = db.SQL().Exec(`
		UPDATE parse_decisions
		   SET jellyfin_identified = 0,
		       metadata_state = 'missing_provider_ids'
		 WHERE id = ?`, unresolved)
	require.NoError(t, err)

	staleEpisode, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	_, err = db.SQL().Exec(`
		UPDATE parse_decisions
		   SET jellyfin_identified = 1,
		       metadata_state = 'series_identified_episode_stale'
		 WHERE id = ?`, staleEpisode)
	require.NoError(t, err)

	require.NoError(t, db.migrate())

	var state, metadataError, nextCheck string
	require.NoError(t, db.SQL().QueryRow(`
		SELECT metadata_state, COALESCE(metadata_error, ''), COALESCE(next_metadata_check_at, '')
		  FROM parse_decisions WHERE id = ?`, contradictory).Scan(&state, &metadataError, &nextCheck))
	require.Equal(t, "identified", state)
	require.Empty(t, metadataError)
	require.Empty(t, nextCheck)

	require.NoError(t, db.SQL().QueryRow(
		`SELECT metadata_state FROM parse_decisions WHERE id = ?`, unresolved).Scan(&state))
	require.Equal(t, "missing_provider_ids", state)

	require.NoError(t, db.SQL().QueryRow(
		`SELECT metadata_state FROM parse_decisions WHERE id = ?`, staleEpisode).Scan(&state))
	require.Equal(t, "series_identified_episode_stale", state)
}
