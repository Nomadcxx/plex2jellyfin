package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/paths"
	_ "modernc.org/sqlite"
)

// MediaDB is the main database handle for JellyWatch media tracking
type MediaDB struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// Open opens or creates the database at the default location
func Open() (*MediaDB, error) {
	dbPath, err := paths.DatabasePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}
	return OpenPath(dbPath)
}

// OpenPath opens or creates the database at a specific path
func OpenPath(path string) (*MediaDB, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open with WAL mode for better concurrent access
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	mdb := &MediaDB{
		db:   db,
		path: path,
	}

	// Apply migrations
	if err := mdb.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return mdb, nil
}

// Close closes the database connection
func (m *MediaDB) Close() error {
	return m.db.Close()
}

// Path returns the filesystem path to the database file
func (m *MediaDB) Path() string {
	return m.path
}

// migrate applies any pending schema migrations
func (m *MediaDB) migrate() error {
	return applyMigrations(m.db)
}

// DB returns the underlying sql.DB for advanced operations
func (m *MediaDB) DB() *sql.DB {
	return m.db
}
