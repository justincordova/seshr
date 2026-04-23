// Package opencode implements backend.SessionStore, backend.LiveDetector, and
// backend.SessionEditor for the OpenCode agent.
//
// OpenCode persists its state in a single SQLite database (drizzle schema).
// On macOS and Linux this lives at ~/.local/share/opencode/opencode.db.
// A Store holds a read connection for the full lifetime of the process; the
// write connection is opened lazily on the first destructive call (Phase 11)
// to avoid holding a write lock just for read traffic.
package opencode

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

// ErrNoDatabase is returned by NewStore when the configured DB path does not
// exist. Callers treat this as "OpenCode is not installed" and skip backend
// registration rather than surfacing a user-visible error.
var ErrNoDatabase = errors.New("opencode database not found")

// connections holds the read + (lazy) write *sql.DB for a single DB file.
type connections struct {
	dbPath string
	read   *sql.DB
	write  *sql.DB // lazily opened; guarded by mu
	mu     sync.Mutex
}

// openRead opens a read-only connection capped at one connection. The
// busy-timeout guards against transient lock contention with OpenCode itself
// or a second seshr process. No foreign keys — read paths don't mutate.
func openRead(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(500)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open opencode read db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping opencode read db: %w", err)
	}
	return db, nil
}

// openWrite lazily constructs the writable handle. foreign_keys=on enforces
// cascades (session → message → part). Serialized to a single connection so
// that BEGIN IMMEDIATE transactions in the editor don't race with themselves.
//
//nolint:unused // used by Phase 11 SessionEditor (prune/delete) on first write
func (c *connections) openWrite() (*sql.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.write != nil {
		return c.write, nil
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)&_pragma=foreign_keys(1)", c.dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open opencode write db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping opencode write db: %w", err)
	}
	c.write = db
	return db, nil
}

// Close releases both connections if they were opened. Safe to call multiple
// times; a second call is a no-op.
func (c *connections) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	if c.read != nil {
		if err := c.read.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close read: %w", err))
		}
		c.read = nil
	}
	if c.write != nil {
		if err := c.write.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close write: %w", err))
		}
		c.write = nil
	}
	return errors.Join(errs...)
}

// DefaultDBPath returns the platform-standard location for opencode.db.
//
// OpenCode follows the XDG data-dir convention on both macOS and Linux, so
// the path is $HOME/.local/share/opencode/opencode.db on both. (macOS does
// NOT use ~/Library/Application Support — verified against a real install.)
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db"), nil
}
