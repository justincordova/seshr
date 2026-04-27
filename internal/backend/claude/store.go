package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Store implements backend.SessionStore for Claude Code JSONL sessions.
type Store struct {
	rootDir string // e.g. ~/.claude/projects or --dir override
}

// NewStore returns a Store rooted at rootDir.
func NewStore(rootDir string) *Store {
	return &Store{rootDir: rootDir}
}

func (s *Store) Kind() session.SourceKind { return session.SourceClaude }

// Scan returns metadata for all Claude Code sessions under rootDir.
func (s *Store) Scan(_ context.Context) ([]backend.SessionMeta, error) {
	metas, err := scanDir(s.rootDir)
	if err != nil {
		return nil, err
	}
	out := make([]backend.SessionMeta, 0, len(metas))
	for _, m := range metas {
		out = append(out, translateMeta(m))
	}
	return out, nil
}

// Load parses the full session file and returns it with a byte-offset cursor.
func (s *Store) Load(ctx context.Context, id string) (*session.Session, backend.Cursor, error) {
	path, err := s.transcriptPath(id)
	if err != nil {
		return nil, backend.Cursor{}, err
	}
	p := NewClaude()
	sess, err := p.Parse(ctx, path)
	if err != nil {
		return nil, backend.Cursor{}, err
	}
	ident, err := fileIdentity(path)
	if err != nil {
		return sess, encodeCursor(cursorData{}), nil
	}
	info, _ := os.Stat(path)
	if info != nil {
		ident.ByteOffset = info.Size()
	}
	return sess, encodeCursor(ident), nil
}

// LoadIncremental reads turns appended since the cursor was captured.
// If the file has been rotated (identity mismatch), falls back to full Load.
func (s *Store) LoadIncremental(ctx context.Context, id string, cur backend.Cursor) ([]session.Turn, backend.Cursor, error) {
	path, err := s.transcriptPath(id)
	if err != nil {
		return nil, cur, err
	}
	current, err := fileIdentity(path)
	if err != nil {
		return nil, cur, err
	}
	prev, err := decodeCursor(cur)
	if err != nil || !identitiesMatch(prev, current) {
		// Fall back to full reload.
		sess, newCur, err := s.Load(ctx, id)
		if err != nil {
			return nil, cur, err
		}
		return sess.Turns, newCur, nil
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, cur, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = fh.Close() }()
	if _, err := fh.Seek(prev.ByteOffset, io.SeekStart); err != nil {
		return nil, cur, fmt.Errorf("seek: %w", err)
	}
	turns, bytesRead, err := parseJSONLStream(fh)
	if err != nil {
		return nil, cur, err
	}
	next := current
	next.ByteOffset = prev.ByteOffset + bytesRead
	return turns, encodeCursor(next), nil
}

// LoadRange loads a slice of turns by index (from inclusive, to exclusive).
func (s *Store) LoadRange(_ context.Context, id string, fromIdx, toIdx int) ([]session.Turn, error) {
	if fromIdx < 0 || toIdx <= fromIdx {
		return nil, fmt.Errorf("invalid range [%d,%d)", fromIdx, toIdx)
	}
	path, err := s.transcriptPath(id)
	if err != nil {
		return nil, err
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = fh.Close() }()
	return parseJSONLRange(fh, fromIdx, toIdx)
}

// Close is a no-op; JSONL files are opened per-read.
func (s *Store) Close() error { return nil }

// transcriptPath locates the .jsonl file for the given session ID under rootDir.
// It walks rootDir/*/<id>.jsonl and returns the first match.
func (s *Store) transcriptPath(id string) (string, error) {
	pattern := filepath.Join(s.rootDir, "*", id+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("session not found: " + id)
	}
	return matches[0], nil
}

// translateMeta converts a claudeMeta to backend.SessionMeta.
func translateMeta(m claudeMeta) backend.SessionMeta {
	return backend.SessionMeta{
		ID:         m.ID,
		Kind:       m.Source,
		Project:    m.Project,
		Directory:  filepath.Dir(m.Path),
		Title:      "",
		TokenCount: m.TokenCount,
		TurnCount:  m.TurnCount,
		CostUSD:    0,
		CreatedAt:  m.ModifiedAt,
		UpdatedAt:  m.ModifiedAt,
		SizeBytes:  m.Size,
		HasBackup:  m.HasBackup,
	}
}
