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
// It blocks other operations on this handle and rolls back all deletes if the
// operation is cancelled or a table cannot be cleared.
func ResetDatabase(ctx context.Context, m *MediaDB, preserve []string, progress chan<- ProgressEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	keep := map[string]bool{}
	for _, t := range preserve {
		keep[t] = true
	}

	tables, err := listUserTables(m.db)
	if err != nil {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reset: %w", err)
	}
	defer tx.Rollback()

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
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", name)); err != nil {
			return fmt.Errorf("truncate %s: %w", name, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}

	progress <- ProgressEvent{Phase: "vacuuming"}
	// SQLite cannot run VACUUM inside the delete transaction. A committed reset
	// stays complete even if vacuuming later fails.
	if _, err := m.db.Exec("VACUUM"); err != nil {
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
