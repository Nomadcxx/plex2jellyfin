package database

import (
	"fmt"
	"time"
)

type DeferredOp struct {
	Path       string
	Type       string
	SourcePath string
	TargetPath string
	Reason     string
	DeferredAt time.Time
}

func (m *MediaDB) SaveDeferredOp(op DeferredOp) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT INTO deferred_ops (path, type, source_path, target_path, reason, deferred_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		op.Path, op.Type, op.SourcePath, op.TargetPath, op.Reason, op.DeferredAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save deferred op: %w", err)
	}
	return nil
}

func (m *MediaDB) DeleteDeferredOpsForPath(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("DELETE FROM deferred_ops WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("failed to delete deferred ops: %w", err)
	}
	return nil
}

func (m *MediaDB) LoadDeferredOps() ([]DeferredOp, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT path, type, source_path, target_path, reason, deferred_at
		 FROM deferred_ops ORDER BY deferred_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load deferred ops: %w", err)
	}
	defer rows.Close()

	var ops []DeferredOp
	for rows.Next() {
		var op DeferredOp
		if err := rows.Scan(&op.Path, &op.Type, &op.SourcePath, &op.TargetPath, &op.Reason, &op.DeferredAt); err != nil {
			return ops, fmt.Errorf("failed to scan deferred op: %w", err)
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
