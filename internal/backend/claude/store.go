package claude

import (
	"context"
	"errors"
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

// Scan delegates to the session package scan and translates to backend.SessionMeta.
func (s *Store) Scan(_ context.Context) ([]backend.SessionMeta, error) {
	metas, err := session.Scan(s.rootDir)
	if err != nil {
		return nil, err
	}
	out := make([]backend.SessionMeta, 0, len(metas))
	for _, m := range metas {
		out = append(out, translateMeta(m))
	}
	return out, nil
}

// Load parses the full session file and returns it with a placeholder cursor.
func (s *Store) Load(ctx context.Context, id string) (*session.Session, backend.Cursor, error) {
	path, err := s.transcriptPath(id)
	if err != nil {
		return nil, backend.Cursor{}, err
	}
	p := session.NewClaude()
	sess, err := p.Parse(ctx, path)
	if err != nil {
		return nil, backend.Cursor{}, err
	}
	cur := backend.Cursor{Kind: session.SourceClaude, Data: []byte("{}")}
	return sess, cur, nil
}

// LoadIncremental is not yet implemented; full implementation is Phase 5.
func (s *Store) LoadIncremental(_ context.Context, _ string, cur backend.Cursor) ([]session.Turn, backend.Cursor, error) {
	return nil, cur, errors.New("not yet implemented")
}

// LoadRange is not yet implemented; full implementation is Phase 5.
func (s *Store) LoadRange(_ context.Context, _ string, _, _ int) ([]session.Turn, error) {
	return nil, errors.New("not yet implemented")
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

// translateMeta converts a session.SessionMeta to backend.SessionMeta.
func translateMeta(m session.SessionMeta) backend.SessionMeta {
	return backend.SessionMeta{
		ID:         m.ID,
		Kind:       m.Source,
		Project:    m.Project,
		Directory:  filepath.Dir(m.Path),
		Title:      "",
		TokenCount: m.TokenCount,
		CostUSD:    0,
		CreatedAt:  m.ModifiedAt, // placeholder; real value in Phase 5
		UpdatedAt:  m.ModifiedAt,
		SizeBytes:  m.Size,
		HasBackup:  m.HasBackup,
	}
}
