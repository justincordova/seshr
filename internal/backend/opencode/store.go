package opencode

import (
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
