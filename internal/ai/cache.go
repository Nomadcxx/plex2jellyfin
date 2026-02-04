package ai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Cache handles caching of AI parse results
type Cache struct {
	db *sql.DB
}

// NewCache creates a new AI cache
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Get retrieves a cached parse result
func (c *Cache) Get(inputNormalized, inputType, model string) (*Result, error) {
	var (
		id           int
		title        string
		year         *int
		mediaType    string
		season       *int
		episodesJSON string
		absoluteEp   *int
		airDate      string
		confidence   float64
		lastUsedAt   time.Time
		usageCount   int
	)

	query := `
		SELECT id, title, year, media_type, season, episodes, absolute_episode,
		       air_date, confidence, last_used_at, usage_count
		FROM ai_parse_cache
		WHERE input_normalized = ? AND input_type = ? AND model = ?
		ORDER BY usage_count DESC, last_used_at DESC
		LIMIT 1
	`

	err := c.db.QueryRow(query, inputNormalized, inputType, model).Scan(
		&id, &title, &year, &mediaType, &season, &episodesJSON,
		&absoluteEp, &airDate, &confidence, &lastUsedAt, &usageCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Parse episodes JSON
	var episodes []int
	if episodesJSON != "" {
		if err := json.Unmarshal([]byte(episodesJSON), &episodes); err != nil {
			return nil, fmt.Errorf("failed to parse episodes JSON: %w", err)
		}
	}

	// Update usage stats asynchronously
	go c.updateUsage(id)

	return &Result{
		Title:           title,
		Year:            NewFlexInt(year),
		Type:            mediaType,
		Season:          NewFlexInt(season),
		Episodes:        episodes,
		AbsoluteEpisode: NewFlexInt(absoluteEp),
		AirDate:         airDate,
		Confidence:      confidence,
	}, nil
}

// Put stores a parse result in the cache
func (c *Cache) Put(inputNormalized, inputType, model string, result *Result, latency time.Duration) error {
	// Serialize episodes to JSON
	episodesJSON, err := json.Marshal(result.Episodes)
	if err != nil {
		return fmt.Errorf("failed to serialize episodes: %w", err)
	}

	query := `
		INSERT INTO ai_parse_cache (
			input_normalized, input_type, title, year, media_type,
			season, episodes, absolute_episode, air_date, confidence,
			model, latency_ms, created_at, last_used_at, usage_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(input_normalized, input_type, model)
		DO UPDATE SET
			title = excluded.title,
			year = excluded.year,
			media_type = excluded.media_type,
			season = excluded.season,
			episodes = excluded.episodes,
			absolute_episode = excluded.absolute_episode,
			air_date = excluded.air_date,
			confidence = excluded.confidence,
			latency_ms = excluded.latency_ms,
			last_used_at = CURRENT_TIMESTAMP,
			usage_count = usage_count + 1
	`

	_, err = c.db.Exec(query,
		inputNormalized, inputType, result.Title, result.Year.Int(), result.Type,
		result.Season.Int(), string(episodesJSON), result.AbsoluteEpisode.Int(), result.AirDate,
		result.Confidence, model, latency.Milliseconds(),
	)

	return err
}

// updateUsage updates the last_used_at and usage_count
func (c *Cache) updateUsage(id int) {
	_, err := c.db.Exec(`
		UPDATE ai_parse_cache
		SET last_used_at = CURRENT_TIMESTAMP, usage_count = usage_count + 1
		WHERE id = ?
	`, id)
	if err != nil {
		// Log but don't fail - this is best-effort tracking
		fmt.Printf("[WARN] Failed to update cache usage: %v\n", err)
	}
}

// Cleanup removes old cache entries (older than 90 days with low usage)
func (c *Cache) Cleanup() (int64, error) {
	result, err := c.db.Exec(`
		DELETE FROM ai_parse_cache
		WHERE last_used_at < datetime('now', '-90 days')
		  AND usage_count < 5
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup cache: %w", err)
	}

	return result.RowsAffected()
}

// GetStats returns cache statistics
func (c *Cache) GetStats() (totalEntries, hitRate int64, err error) {
	// Total entries
	err = c.db.QueryRow("SELECT COUNT(*) FROM ai_parse_cache").Scan(&totalEntries)
	if err != nil {
		return 0, 0, err
	}

	// Average usage
	var avgUsage sql.NullFloat64
	err = c.db.QueryRow("SELECT AVG(usage_count) FROM ai_parse_cache").Scan(&avgUsage)
	if err != nil {
		return 0, 0, err
	}

	if avgUsage.Valid {
		hitRate = int64(avgUsage.Float64)
	}

	return
}

// NormalizeInput creates a cache key from input string
func NormalizeInput(input string) string {
	return input // Simple for now - could add normalization in future
}
