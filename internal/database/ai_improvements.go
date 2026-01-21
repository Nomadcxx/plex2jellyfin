package database

import (
	"database/sql"
	"time"
)

// AIImprovement represents an AI enhancement request stored in the database
type AIImprovement struct {
	ID        int64
	RequestID string
	Filename  string

	UserTitle string
	UserType  string
	UserYear  *int

	AITitle      *string
	AIType       *string
	AIYear       *int
	AIConfidence *float64

	Status       string
	Attempts     int
	ErrorMessage *string

	Model           *string
	OriginalRequest *string

	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

// UpsertAIImprovement inserts or updates an AI improvement request
func (m *MediaDB) UpsertAIImprovement(imp *AIImprovement) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	imp.UpdatedAt = time.Now()

	var existingID int64
	err := m.db.QueryRow(
		`SELECT id FROM ai_improvements WHERE request_id = ?`,
		imp.RequestID,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		if imp.CreatedAt.IsZero() {
			imp.CreatedAt = time.Now()
		}

		result, err := m.db.Exec(`
			INSERT INTO ai_improvements (
				request_id, filename, user_title, user_type, user_year,
				ai_title, ai_type, ai_year, ai_confidence,
				status, attempts, error_message,
				model, original_request, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			imp.RequestID, imp.Filename, imp.UserTitle, imp.UserType, imp.UserYear,
			imp.AITitle, imp.AIType, imp.AIYear, imp.AIConfidence,
			imp.Status, imp.Attempts, imp.ErrorMessage,
			imp.Model, imp.OriginalRequest, imp.CreatedAt, imp.UpdatedAt,
		)
		if err != nil {
			return err
		}

		imp.ID, _ = result.LastInsertId()
		return nil
	}

	if err != nil {
		return err
	}

	_, err = m.db.Exec(`
		UPDATE ai_improvements SET
			filename = ?, user_title = ?, user_type = ?, user_year = ?,
			ai_title = ?, ai_type = ?, ai_year = ?, ai_confidence = ?,
			status = ?, attempts = ?, error_message = ?,
			model = ?, original_request = ?, updated_at = ?,
			completed_at = COALESCE(?, completed_at)
		WHERE id = ?`,
		imp.Filename, imp.UserTitle, imp.UserType, imp.UserYear,
		imp.AITitle, imp.AIType, imp.AIYear, imp.AIConfidence,
		imp.Status, imp.Attempts, imp.ErrorMessage,
		imp.Model, imp.OriginalRequest, imp.UpdatedAt, imp.CompletedAt,
		existingID,
	)

	imp.ID = existingID
	return err
}

// GetAIImprovement retrieves an AI improvement by request ID
func (m *MediaDB) GetAIImprovement(requestID string) (*AIImprovement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var imp AIImprovement
	err := m.db.QueryRow(`
		SELECT id, request_id, filename, user_title, user_type, user_year,
		       ai_title, ai_type, ai_year, ai_confidence,
		       status, attempts, error_message,
		       model, original_request, created_at, updated_at, completed_at
		FROM ai_improvements
		WHERE request_id = ?`,
		requestID,
	).Scan(
		&imp.ID, &imp.RequestID, &imp.Filename, &imp.UserTitle, &imp.UserType, &imp.UserYear,
		&imp.AITitle, &imp.AIType, &imp.AIYear, &imp.AIConfidence,
		&imp.Status, &imp.Attempts, &imp.ErrorMessage,
		&imp.Model, &imp.OriginalRequest, &imp.CreatedAt, &imp.UpdatedAt, &imp.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &imp, err
}

// GetPendingAIImprovements retrieves all pending AI improvements
func (m *MediaDB) GetPendingAIImprovements(limit int) ([]*AIImprovement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id, request_id, filename, user_title, user_type, user_year,
		       ai_title, ai_type, ai_year, ai_confidence,
		       status, attempts, error_message,
		       model, original_request, created_at, updated_at, completed_at
		FROM ai_improvements
		WHERE status = ?
		ORDER BY created_at ASC
		LIMIT ?`

	rows, err := m.db.Query(query, "pending", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var improvements []*AIImprovement
	for rows.Next() {
		var imp AIImprovement
		err := rows.Scan(
			&imp.ID, &imp.RequestID, &imp.Filename, &imp.UserTitle, &imp.UserType, &imp.UserYear,
			&imp.AITitle, &imp.AIType, &imp.AIYear, &imp.AIConfidence,
			&imp.Status, &imp.Attempts, &imp.ErrorMessage,
			&imp.Model, &imp.OriginalRequest, &imp.CreatedAt, &imp.UpdatedAt, &imp.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		improvements = append(improvements, &imp)
	}

	return improvements, rows.Err()
}

// UpdateAIImprovementStatus updates the status of an AI improvement
func (m *MediaDB) UpdateAIImprovementStatus(requestID string, status string, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `UPDATE ai_improvements SET status = ?, updated_at = ?`
	args := []interface{}{status, time.Now()}

	if errorMsg != "" {
		query += `, error_message = ?, attempts = attempts + 1`
		args = append(args, errorMsg)
	} else {
		query += `, error_message = NULL`
	}

	if status == "completed" {
		query += `, completed_at = ?`
		args = append(args, time.Now())
	}

	query += ` WHERE request_id = ?`
	args = append(args, requestID)

	_, err := m.db.Exec(query, args...)
	return err
}

// DeleteAIImprovement removes an AI improvement by request ID
func (m *MediaDB) DeleteAIImprovement(requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM ai_improvements WHERE request_id = ?`, requestID)
	return err
}

// GetAIImprovementsByModel retrieves all AI improvements for a specific model
func (m *MediaDB) GetAIImprovementsByModel(model string, limit int) ([]*AIImprovement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := m.db.Query(`
		SELECT id, request_id, filename, user_title, user_type, user_year,
		       ai_title, ai_type, ai_year, ai_confidence,
		       status, attempts, error_message,
		       model, original_request, created_at, updated_at, completed_at
		FROM ai_improvements
		WHERE model = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		model, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var improvements []*AIImprovement
	for rows.Next() {
		var imp AIImprovement
		err := rows.Scan(
			&imp.ID, &imp.RequestID, &imp.Filename, &imp.UserTitle, &imp.UserType, &imp.UserYear,
			&imp.AITitle, &imp.AIType, &imp.AIYear, &imp.AIConfidence,
			&imp.Status, &imp.Attempts, &imp.ErrorMessage,
			&imp.Model, &imp.OriginalRequest, &imp.CreatedAt, &imp.UpdatedAt, &imp.CompletedAt,
		)
		if err != nil {
			return nil, err
		}
		improvements = append(improvements, &imp)
	}

	return improvements, rows.Err()
}

// CountAIImprovementsByStatus returns the count of AI improvements by status
func (m *MediaDB) CountAIImprovementsByStatus(status string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM ai_improvements WHERE status = ?`, status).Scan(&count)
	return count, err
}
