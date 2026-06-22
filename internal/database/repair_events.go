package database

import (
	"database/sql"
	"fmt"
	"time"
)

type RepairEvent struct {
	ID           int64     `json:"id"`
	EventAt      time.Time `json:"event_at"`
	Action       string    `json:"action"`
	SafetyClass  string    `json:"safety_class"`
	Confidence   float64   `json:"confidence"`
	SourcePath   string    `json:"source_path,omitempty"`
	TargetPath   string    `json:"target_path,omitempty"`
	Outcome      string    `json:"outcome"`
	Error        string    `json:"error,omitempty"`
	LLMConsulted bool      `json:"llm_consulted"`
	EvidenceJSON string    `json:"evidence_json,omitempty"`
}

func (m *MediaDB) InsertRepairEvent(e RepairEvent) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.EventAt.IsZero() {
		e.EventAt = time.Now().UTC()
	}
	res, err := m.db.Exec(`
		INSERT INTO repair_events
			(event_at, action, safety_class, confidence, source_path, target_path,
			 outcome, error, llm_consulted, evidence_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventAt, e.Action, e.SafetyClass, e.Confidence, nullStr(e.SourcePath),
		nullStr(e.TargetPath), e.Outcome, nullStr(e.Error), e.LLMConsulted,
		nullStr(e.EvidenceJSON))
	if err != nil {
		return 0, fmt.Errorf("InsertRepairEvent: %w", err)
	}
	return res.LastInsertId()
}

func (m *MediaDB) ListRepairEventsSince(since time.Time, limit int) ([]RepairEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	rows, err := m.db.Query(`
		SELECT id, event_at, action, safety_class, confidence, source_path,
		       target_path, outcome, error, llm_consulted, evidence_json
		  FROM repair_events
		 WHERE event_at >= ?
		 ORDER BY event_at DESC
		 LIMIT ?`, since, limit)
	if err != nil {
		return nil, fmt.Errorf("ListRepairEventsSince: %w", err)
	}
	defer rows.Close()

	var out []RepairEvent
	for rows.Next() {
		var e RepairEvent
		var source, target, errMsg, evidence sql.NullString
		if err := rows.Scan(&e.ID, &e.EventAt, &e.Action, &e.SafetyClass,
			&e.Confidence, &source, &target, &e.Outcome, &errMsg,
			&e.LLMConsulted, &evidence); err != nil {
			return nil, fmt.Errorf("ListRepairEventsSince scan: %w", err)
		}
		e.SourcePath = source.String
		e.TargetPath = target.String
		e.Error = errMsg.String
		e.EvidenceJSON = evidence.String
		out = append(out, e)
	}
	return out, rows.Err()
}
