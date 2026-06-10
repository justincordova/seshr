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
	offset, err := lastRecordBoundary(path)
	if err != nil {
		// Without a trustworthy boundary offset we cannot set ByteOffset.
		// Emitting the identity fields with ByteOffset==0 would make the next
		// LoadIncremental match identity and seek to offset 0, re-reading the
		// whole file and duplicating every turn. Return a cold cursor so the
		// next incremental load takes the clean full-reload path instead.
		return sess, encodeCursor(cursorData{}), nil
	}
	// Anchor ByteOffset to the last newline-terminated record boundary rather
	// than info.Size(). If the live agent is mid-write when Load runs, the
	// file ends with a partial line whose bytes are NOT yet a complete record;
	// pointing ByteOffset past them would make the next LoadIncremental seek
	// into the middle of that record once the writer completes it, losing the
	// turn. parseJSONLStream uses the same boundary accounting.
	ident.ByteOffset = offset
	return sess, encodeCursor(ident), nil
}

// lastRecordBoundary returns the byte offset immediately after the final
// newline in the file — i.e. the start of any partial trailing record a live
// writer may still be appending. For a file whose last record is properly
// newline-terminated this equals the file size. An empty file returns 0.
func lastRecordBoundary(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	size := info.Size()
	if size == 0 {
		return 0, nil
	}

	const chunk = 64 * 1024
	buf := make([]byte, chunk)
	pos := size
	for pos > 0 {
		readLen := int64(chunk)
		if pos < readLen {
			readLen = pos
		}
		start := pos - readLen
		if _, err := f.ReadAt(buf[:readLen], start); err != nil && !errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("read %s: %w", path, err)
		}
		for i := int(readLen) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				return start + int64(i) + 1, nil
			}
		}
		pos = start
	}
	// No newline anywhere: the whole file is one partial record.
	return 0, nil
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
		// Rotation/truncation/cold cursor: the byte offset means nothing
		// against the current file. Returning the full turn list here would
		// make append-semantics callers duplicate every turn, so surface the
		// sentinel and let the caller rebuild from a clean Load.
		return nil, cur, backend.ErrCursorInvalid
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
//
// The range is taken from a full Parse so it uses the SAME turn-index space
// as Load: Parse folds attached tool_result records into their assistant
// turn, so a line-level scan that skips that attachment would count attached
// results as standalone turns and drift right of Load's indices by one per
// attachment — returning earlier turns than the caller asked for.
func (s *Store) LoadRange(ctx context.Context, id string, fromIdx, toIdx int) ([]session.Turn, error) {
	if fromIdx < 0 || toIdx <= fromIdx {
		return nil, fmt.Errorf("invalid range [%d,%d)", fromIdx, toIdx)
	}
	path, err := s.transcriptPath(id)
	if err != nil {
		return nil, err
	}
	sess, err := NewClaude().Parse(ctx, path)
	if err != nil {
		return nil, err
	}
	if fromIdx >= len(sess.Turns) {
		return nil, nil
	}
	if toIdx > len(sess.Turns) {
		toIdx = len(sess.Turns)
	}
	return sess.Turns[fromIdx:toIdx], nil
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

// backupPath locates the .bak file for the given session ID under rootDir,
// returning its path even if the original .jsonl no longer exists (e.g.,
// after a Delete). The corresponding .jsonl path can be derived by trimming
// the ".bak" suffix.
func (s *Store) backupPath(id string) (string, error) {
	pattern := filepath.Join(s.rootDir, "*", id+".jsonl.bak")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("backup not found: " + id)
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
