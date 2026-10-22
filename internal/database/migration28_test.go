package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigration28AddsRetryMetadataAndPreservesFailures(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.SQL().Query(`PRAGMA table_info(parse_decisions)`)
	require.NoError(t, err)
	hasRetryDestination := false
	hasRetryRequiredBytes := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		require.NoError(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey))
		if name == "retry_destination" {
			hasRetryDestination = true
		}
		if name == "retry_required_bytes" {
			hasRetryRequiredBytes = true
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	if hasRetryDestination {
		_, err = db.SQL().Exec(`ALTER TABLE parse_decisions DROP COLUMN retry_destination`)
		require.NoError(t, err)
	}
	if hasRetryRequiredBytes {
		_, err = db.SQL().Exec(`ALTER TABLE parse_decisions DROP COLUMN retry_required_bytes`)
		require.NoError(t, err)
	}
	_, err = db.SQL().Exec(`DELETE FROM schema_version WHERE version >= 28`)
	require.NoError(t, err)

	res, err := db.SQL().Exec(`
		INSERT INTO parse_decisions (source_path, source_filename, source_size, event_at, organize_outcome, organize_error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"/downloads/movie.mkv", "movie.mkv", 1234, time.Now().UTC(), "failed", "insufficient space")
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)

	require.NoError(t, db.migrate())

	var sourceSize, retryRequiredBytes int64
	var destination string
	require.NoError(t, db.SQL().QueryRow(
		`SELECT source_size, COALESCE(retry_destination, ''), retry_required_bytes FROM parse_decisions WHERE id = ?`, id,
	).Scan(&sourceSize, &destination, &retryRequiredBytes))
	require.Equal(t, int64(1234), sourceSize)
	require.Empty(t, destination)
	require.Zero(t, retryRequiredBytes)
	var version int
	require.NoError(t, db.SQL().QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version))
	require.Equal(t, 28, version)
}
