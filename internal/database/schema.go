package database

import "database/sql"

// Schema version for migrations
const currentSchemaVersion = 12

// SQL migration scripts
var migrations = []migration{
	{
		version: 1,
		up: []string{
			// Series table
			`CREATE TABLE series (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				
				-- Identification
				title TEXT NOT NULL,
				title_normalized TEXT NOT NULL,
				year INTEGER,
				
				-- External IDs (nullable - may not have Sonarr)
				tvdb_id INTEGER,
				imdb_id TEXT,
				sonarr_id INTEGER,
				
				-- Location
				canonical_path TEXT NOT NULL,
				library_root TEXT NOT NULL,
				
				-- Source tracking
				source TEXT NOT NULL DEFAULT 'jellywatch',
				source_priority INTEGER NOT NULL DEFAULT 0,
				
				-- Stats
				episode_count INTEGER DEFAULT 0,
				last_episode_added DATETIME,
				
				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_synced_at DATETIME,
				
				-- Constraints
				UNIQUE(title_normalized, year)
			)`,

			// Series indexes
			`CREATE INDEX idx_series_normalized ON series(title_normalized)`,
			`CREATE INDEX idx_series_normalized_year ON series(title_normalized, year)`,
			`CREATE INDEX idx_series_tvdb ON series(tvdb_id)`,
			`CREATE INDEX idx_series_sonarr ON series(sonarr_id)`,
			`CREATE INDEX idx_series_library ON series(library_root)`,

			// Movies table
			`CREATE TABLE movies (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				
				-- Identification
				title TEXT NOT NULL,
				title_normalized TEXT NOT NULL,
				year INTEGER,
				
				-- External IDs
				tmdb_id INTEGER,
				imdb_id TEXT,
				radarr_id INTEGER,
				
				-- Location
				canonical_path TEXT NOT NULL,
				library_root TEXT NOT NULL,
				
				-- Source tracking
				source TEXT NOT NULL DEFAULT 'jellywatch',
				source_priority INTEGER NOT NULL DEFAULT 0,
				
				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_synced_at DATETIME,
				
				-- Constraints
				UNIQUE(title_normalized, year)
			)`,

			// Movies indexes
			`CREATE INDEX idx_movies_normalized ON movies(title_normalized)`,
			`CREATE INDEX idx_movies_normalized_year ON movies(title_normalized, year)`,
			`CREATE INDEX idx_movies_tmdb ON movies(tmdb_id)`,
			`CREATE INDEX idx_movies_radarr ON movies(radarr_id)`,
			`CREATE INDEX idx_movies_library ON movies(library_root)`,

			// Aliases table
			`CREATE TABLE aliases (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				alias_normalized TEXT NOT NULL,
				media_type TEXT NOT NULL,
				media_id INTEGER NOT NULL,
				
				UNIQUE(alias_normalized, media_type)
			)`,

			`CREATE INDEX idx_aliases_lookup ON aliases(alias_normalized, media_type)`,

			// Conflicts table
			`CREATE TABLE conflicts (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				media_type TEXT NOT NULL,
				title TEXT NOT NULL,
				title_normalized TEXT NOT NULL,
				year INTEGER,
				
				-- JSON array of locations
				locations TEXT NOT NULL,
				
				-- Resolution
				resolved BOOLEAN DEFAULT FALSE,
				resolved_at DATETIME,
				resolved_path TEXT,
				
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,

			`CREATE INDEX idx_conflicts_unresolved ON conflicts(resolved, media_type)`,

			// Sync log table
			`CREATE TABLE sync_log (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				source TEXT NOT NULL,
				started_at DATETIME NOT NULL,
				completed_at DATETIME,
				status TEXT NOT NULL,
				items_processed INTEGER DEFAULT 0,
				items_added INTEGER DEFAULT 0,
				items_updated INTEGER DEFAULT 0,
				error_message TEXT
			)`,

			`CREATE INDEX idx_sync_log_source ON sync_log(source, started_at DESC)`,

			// Schema version table
			`CREATE TABLE schema_version (
				version INTEGER PRIMARY KEY,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,

			`INSERT INTO schema_version (version) VALUES (1)`,
		},
	},
	{
		version: 2,
		up: []string{
			// AI parse cache table
			`CREATE TABLE ai_parse_cache (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				
				-- Input fingerprint
				input_normalized TEXT NOT NULL,
				input_type TEXT NOT NULL,
				
				-- Parsed result
				title TEXT NOT NULL,
				year INTEGER,
				media_type TEXT NOT NULL,
				season INTEGER,
				episodes TEXT,
				absolute_episode INTEGER,
				air_date TEXT,
				confidence REAL NOT NULL,
				
				-- Metadata
				model TEXT NOT NULL,
				latency_ms INTEGER,
				
				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				last_used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				usage_count INTEGER DEFAULT 1,
				
				-- Constraints
				UNIQUE(input_normalized, input_type, model)
			)`,

			// Cache indexes
			`CREATE INDEX idx_ai_cache_lookup ON ai_parse_cache(input_normalized, input_type)`,
			`CREATE INDEX idx_ai_cache_last_used ON ai_parse_cache(last_used_at DESC)`,
			`CREATE INDEX idx_ai_cache_model ON ai_parse_cache(model)`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (2)`,
		},
	},
	{
		version: 3,
		up: []string{
			// Episodes table - links series to individual episode entries
			`CREATE TABLE episodes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				series_id INTEGER NOT NULL,
				season INTEGER NOT NULL,
				episode INTEGER NOT NULL,
				title TEXT,

				-- Points to the highest quality file for this episode
				best_file_id INTEGER,

				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

				FOREIGN KEY (series_id) REFERENCES series(id) ON DELETE CASCADE,
				UNIQUE(series_id, season, episode)
			)`,

			// Episode indexes
			`CREATE INDEX idx_episodes_series ON episodes(series_id)`,
			`CREATE INDEX idx_episodes_season ON episodes(series_id, season)`,
			`CREATE INDEX idx_episodes_best_file ON episodes(best_file_id)`,

			// Add best_file_id to movies table
			`ALTER TABLE movies ADD COLUMN best_file_id INTEGER`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (3)`,
		},
	},
	{
		version: 4,
		up: []string{
			// Media files table - core of CONDOR system
			`CREATE TABLE media_files (
				id INTEGER PRIMARY KEY AUTOINCREMENT,

				-- File location and metadata
				path TEXT NOT NULL UNIQUE,
				size INTEGER NOT NULL,
				modified_at DATETIME,

				-- Classification
				media_type TEXT NOT NULL CHECK(media_type IN ('movie', 'episode')),
				parent_movie_id INTEGER,
				parent_series_id INTEGER,
				parent_episode_id INTEGER,

				-- Normalized identity (for duplicate detection)
				normalized_title TEXT NOT NULL,
				year INTEGER,
				season INTEGER,
				episode INTEGER,

				-- Quality metadata
				resolution TEXT,
				source_type TEXT,
				codec TEXT,
				audio_format TEXT,
				quality_score INTEGER NOT NULL DEFAULT 0,

				-- Jellyfin compliance
				is_jellyfin_compliant BOOLEAN NOT NULL DEFAULT 0,
				compliance_issues TEXT,

				-- Provenance
				source TEXT NOT NULL DEFAULT 'filesystem',
				source_priority INTEGER NOT NULL DEFAULT 50,
				library_root TEXT,

				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

				FOREIGN KEY (parent_movie_id) REFERENCES movies(id) ON DELETE SET NULL,
				FOREIGN KEY (parent_series_id) REFERENCES series(id) ON DELETE SET NULL,
				FOREIGN KEY (parent_episode_id) REFERENCES episodes(id) ON DELETE SET NULL
			)`,

			// Media files indexes for common queries
			`CREATE INDEX idx_media_files_normalized ON media_files(normalized_title, year, season, episode)`,
			`CREATE INDEX idx_media_files_media_type ON media_files(media_type)`,
			`CREATE INDEX idx_media_files_compliance ON media_files(is_jellyfin_compliant)`,
			`CREATE INDEX idx_media_files_quality ON media_files(quality_score DESC)`,
			`CREATE INDEX idx_media_files_library ON media_files(library_root)`,
			`CREATE INDEX idx_media_files_parent_movie ON media_files(parent_movie_id)`,
			`CREATE INDEX idx_media_files_parent_series ON media_files(parent_series_id)`,
			`CREATE INDEX idx_media_files_parent_episode ON media_files(parent_episode_id)`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (4)`,
		},
	},
	{
		version: 5,
		up: []string{
			// Consolidation plans table - generated plans awaiting execution
			`CREATE TABLE consolidation_plans (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

				-- Status tracking
				status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'executing', 'completed', 'failed', 'skipped')),

				-- Action details
				action TEXT NOT NULL CHECK(action IN ('delete', 'move', 'rename')),
				source_file_id INTEGER NOT NULL,
				source_path TEXT NOT NULL,
				target_path TEXT,

				-- Reasoning
				reason TEXT NOT NULL,
				reason_details TEXT,

				-- Execution tracking
				executed_at DATETIME,
				error_message TEXT,

				FOREIGN KEY (source_file_id) REFERENCES media_files(id) ON DELETE CASCADE
			)`,

			// Consolidation plans indexes
			`CREATE INDEX idx_consolidation_status ON consolidation_plans(status)`,
			`CREATE INDEX idx_consolidation_action ON consolidation_plans(action)`,
			`CREATE INDEX idx_consolidation_created ON consolidation_plans(created_at DESC)`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (5)`,
		},
	},
	{
		version: 6,
		up: []string{
			// AI improvements table - stores user feedback for AI model enhancement
			`CREATE TABLE ai_improvements (
				id INTEGER PRIMARY KEY AUTOINCREMENT,

				-- Request identification
				request_id TEXT NOT NULL UNIQUE,
				filename TEXT NOT NULL,

				-- User-provided correct answer
				user_title TEXT NOT NULL,
				user_type TEXT NOT NULL CHECK(user_type IN ('movie', 'tv')),
				user_year INTEGER,

				-- Original AI result (for comparison)
				ai_title TEXT,
				ai_type TEXT CHECK(ai_type IN ('movie', 'tv')),
				ai_year INTEGER,
				ai_confidence REAL,

				-- Processing status
				status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'processing', 'completed', 'failed')),
				attempts INTEGER DEFAULT 0,

				-- Error tracking
				error_message TEXT,

				-- Metadata
				model TEXT,
				original_request TEXT,

				-- Timestamps
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				completed_at DATETIME
			)`,

			// AI improvements indexes
			`CREATE INDEX idx_ai_improvements_status ON ai_improvements(status)`,
			`CREATE INDEX idx_ai_improvements_model ON ai_improvements(model)`,
			`CREATE INDEX idx_ai_improvements_created ON ai_improvements(created_at DESC)`,
			`CREATE INDEX idx_ai_improvements_confidence ON ai_improvements(ai_confidence)`,

			// Update schema version
			`INSERT INTO schema_version (version) VALUES (6)`,
		},
	},
	{
		version: 7,
		up: []string{
			`CREATE TABLE consolidation_plans_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,

				-- Status tracking
				status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'executing', 'completed', 'failed', 'skipped')),

				-- Action details
				action TEXT NOT NULL CHECK(action IN ('delete', 'move', 'rename')),
				source_file_id INTEGER,
				source_path TEXT NOT NULL,
				target_path TEXT,

				-- Reasoning
				reason TEXT NOT NULL,
				reason_details TEXT,

				-- Execution tracking
				executed_at DATETIME,
				error_message TEXT,

				FOREIGN KEY (source_file_id) REFERENCES media_files(id) ON DELETE CASCADE
			)`,

			`INSERT INTO consolidation_plans_new 
			 (id, created_at, status, action, source_file_id, source_path, target_path, reason, reason_details, executed_at, error_message)
			 SELECT id, created_at, status, action, source_file_id, source_path, target_path, reason, reason_details, executed_at, error_message 
			 FROM consolidation_plans`,

			`DROP TABLE consolidation_plans`,

			`ALTER TABLE consolidation_plans_new RENAME TO consolidation_plans`,

			`CREATE INDEX idx_consolidation_status ON consolidation_plans(status)`,
			`CREATE INDEX idx_consolidation_action ON consolidation_plans(action)`,
			`CREATE INDEX idx_consolidation_created ON consolidation_plans(created_at DESC)`,

			`INSERT INTO schema_version (version) VALUES (7)`,
		},
	},
	{
		version: 8,
		up: []string{
			`ALTER TABLE consolidation_plans ADD COLUMN conflict_id INTEGER REFERENCES conflicts(id)`,
			`INSERT INTO schema_version (version) VALUES (8)`,
		},
	},
	{
		version: 9,
		up: []string{
			// Migration removed - plans are now stored in JSON files
			`INSERT INTO schema_version (version) VALUES (9)`,
		},
	},
	{
		version: 10,
		up: []string{
			// Add confidence tracking to media_files
			`ALTER TABLE media_files ADD COLUMN confidence REAL DEFAULT 1.0`,
			`ALTER TABLE media_files ADD COLUMN parse_method TEXT DEFAULT 'regex'`,
			`ALTER TABLE media_files ADD COLUMN needs_review BOOLEAN NOT NULL DEFAULT 0`,
			`CREATE INDEX idx_media_files_confidence ON media_files(confidence)`,
			`CREATE INDEX idx_media_files_needs_review ON media_files(needs_review)`,
			`INSERT INTO schema_version (version) VALUES (10)`,
		},
	},
	{
		version: 11,
		up: []string{
			// Add sync tracking dirty flags for Sonarr/Radarr integration
			`ALTER TABLE series ADD COLUMN sonarr_synced_at DATETIME`,
			`ALTER TABLE series ADD COLUMN sonarr_path_dirty BOOLEAN DEFAULT 0`,
			`ALTER TABLE series ADD COLUMN radarr_synced_at DATETIME`,
			`ALTER TABLE series ADD COLUMN radarr_path_dirty BOOLEAN DEFAULT 0`,
			`ALTER TABLE movies ADD COLUMN radarr_synced_at DATETIME`,
			`ALTER TABLE movies ADD COLUMN radarr_path_dirty BOOLEAN DEFAULT 0`,
			`INSERT INTO schema_version (version) VALUES (11)`,
		},
	},
	{
		version: 12,
		up: []string{
			// Track Jellyfin-confirmed items keyed by file path.
			`CREATE TABLE jellyfin_items (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				path TEXT NOT NULL UNIQUE,
				jellyfin_item_id TEXT NOT NULL,
				item_name TEXT,
				item_type TEXT,
				confirmed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX idx_jellyfin_items_item_id ON jellyfin_items(jellyfin_item_id)`,
			`INSERT INTO schema_version (version) VALUES (12)`,
		},
	},
}

type migration struct {
	version int
	up      []string
}

// applyMigrations applies any pending schema migrations
func applyMigrations(db *sql.DB) error {
	// Check current version
	var currentVersion int
	err := db.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&currentVersion)
	if err != nil {
		// schema_version doesn't exist yet - this is a fresh database
		currentVersion = 0
	}

	// Apply migrations in order
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		// Begin transaction for this migration
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		// Execute all statements in this migration
		for _, stmt := range m.up {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return err
			}
		}

		// Note: schema_version is updated by the migration's own SQL statements
		// No need to insert again here - each migration handles its own version insert

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
