package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ProgressEvent is emitted by long-running maintenance operations.
type ProgressEvent struct {
	Phase   string
	Msg     string
	Current int
	Total   int
}

// ResetDatabase truncates every user table except those named in preserve.
// Emits ProgressEvent values to progress as it works. Honors ctx cancel
// at table boundaries.
func ResetDatabase(ctx context.Context, db *sql.DB, preserve []string, progress chan<- ProgressEvent) error {
	keep := map[string]bool{}
	for _, t := range preserve {
		keep[t] = true
	}

	tables, err := listUserTables(db)
	if err != nil {
		return err
	}
	progress <- ProgressEvent{Phase: "preparing", Total: len(tables)}

	for i, name := range tables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if keep[name] {
			progress <- ProgressEvent{Phase: "preserving", Msg: name, Current: i + 1, Total: len(tables)}
			continue
		}
		progress <- ProgressEvent{Phase: "truncating", Msg: name, Current: i + 1, Total: len(tables)}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", name)); err != nil {
			return fmt.Errorf("truncate %s: %w", name, err)
		}
	}

	progress <- ProgressEvent{Phase: "vacuuming"}
	// VACUUM cannot run inside a transaction; database/sql + go-sqlite3 does
	// not auto-wrap Exec in one, so this is safe. Use Exec (not ExecContext)
	// because VACUUM ignores cancel mid-statement anyway.
	if _, err := db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}

	progress <- ProgressEvent{Phase: "complete"}
	return nil
}

func listUserTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		if strings.HasPrefix(n, "sqlite_") {
			continue
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
