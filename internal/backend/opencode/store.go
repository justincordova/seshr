package opencode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Compile-time interface assertion.
var _ backend.SessionStore = (*Store)(nil)

// Store implements backend.SessionStore for OpenCode's SQLite database.
type Store struct {
	conns     *connections
	backupDir string
}

// NewStore opens a read connection to dbPath. Returns ErrNoDatabase when
// dbPath does not exist so main can decide not to register the backend at
// all (user has no OC install). backupDir is used by Scan to determine the
// HasBackup flag; empty string disables backup lookups.
func NewStore(dbPath, backupDir string) (*Store, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoDatabase
		}
		return nil, fmt.Errorf("stat %s: %w", dbPath, err)
	}
	read, err := openRead(dbPath)
	if err != nil {
		return nil, err
	}
	return &Store{
		conns: &connections{
			dbPath: dbPath,
			read:   read,
		},
		backupDir: backupDir,
	}, nil
}

// Kind returns SourceOpenCode.
func (s *Store) Kind() session.SourceKind { return session.SourceOpenCode }

// Close releases the DB connection(s).
func (s *Store) Close() error {
	if s.conns == nil {
		return nil
	}
	return s.conns.Close()
}

// DBPath returns the DB file the store was opened against. Used by the
// editor (Phase 11) to derive backup paths.
func (s *Store) DBPath() string { return s.conns.dbPath }

// BackupDir returns the per-user backup root for OC sessions.
func (s *Store) BackupDir() string { return s.backupDir }

// hasBackup returns true when ~/.seshr/backups/opencode/<id>/ exists and has
// at least one *.json file. Called per-session during Scan; cheap because
// scanning a shallow dir is a single syscall on macOS + Linux.
func (s *Store) hasBackup(id string) bool {
	if s.backupDir == "" {
		return false
	}
	dir := filepath.Join(s.backupDir, id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			return true
		}
	}
	return false
}

// LoadIncremental tails a session using a (time_created, id) cursor.
// Populated in Phase 10; Phase 8 returns nil, cur, nil so the TUI fast-tick
// path doesn't crash when it encounters an OC session.
func (s *Store) LoadIncremental(_ context.Context, _ string, cur backend.Cursor) ([]session.Turn, backend.Cursor, error) {
	return nil, cur, nil
}

// LoadRange returns turns [from, to) from the session's current chain.
// Populated in Phase 10; Phase 8 returns an empty slice (not an error) so
// the memory-window eviction path in the TUI degrades to no-op for OC.
func (s *Store) LoadRange(_ context.Context, _ string, _, _ int) ([]session.Turn, error) {
	return nil, nil
}
