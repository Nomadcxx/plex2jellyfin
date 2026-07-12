package database

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	_ "modernc.org/sqlite"
)

// MediaDB is the main database handle for Plex2Jellyfin media tracking
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

	// Open with WAL mode and a real modernc.org/sqlite busy timeout so a
	// manual CLI scan can coexist with daemon housekeeping/scheduler writes.
	db, err := sql.Open("sqlite", sqliteDSN(path))
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

	// Root daemon (SUDO_USER set) creates DB/WAL as root; hand ownership back
	// so the interactive user can run `plex2jellyfin scan` without sudo.
	_ = chownDBArtifacts(path)

	return mdb, nil
}

func chownDBArtifacts(path string) error {
	return paths.ChownToActualUser(path, path+"-wal", path+"-shm", filepath.Dir(path))
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(30000)")
	values.Add("_pragma", "journal_mode(WAL)")
	return path + "?" + values.Encode()
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

// SQL returns the underlying *sql.DB. Preferred over DB() for new code that
// wires the raw handle into maintenance / DDL helpers.
func (m *MediaDB) SQL() *sql.DB {
	return m.db
}
