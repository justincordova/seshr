package claude

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/editor"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
)

// Editor implements backend.SessionEditor for Claude Code sessions.
type Editor struct {
	store *Store
}

// NewEditor returns an Editor backed by the given Store.
func NewEditor(store *Store) *Editor {
	return &Editor{store: store}
}

func (e *Editor) Kind() session.SourceKind { return session.SourceClaude }

// Prune expands the selection using tool-pairing logic, then rewrites the JSONL.
func (e *Editor) Prune(ctx context.Context, id string, sel backend.Selection) (backend.PruneResult, error) {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return backend.PruneResult{}, err
	}

	p := NewClaude()
	sess, err := p.Parse(ctx, path)
	if err != nil {
		return backend.PruneResult{}, err
	}

	// Build an editor.Selection from the backend.Selection turn indices.
	edSel := editor.Selection{Turns: make(map[int]bool, len(sel.TurnIndices))}
	for _, idx := range sel.TurnIndices {
		edSel.Turns[idx] = true
	}

	// Expand to include pairing partners (tool_use ↔ tool_result, user ↔ assistant).
	ts := topics.Cluster(sess, topics.DefaultOptions())
	expanded := editor.ExpandSelection(sess, ts, edSel)

	if err := editor.PruneSession(sess, expanded); err != nil {
		return backend.PruneResult{}, err
	}

	return backend.PruneResult{}, nil
}

// Delete removes the JSONL file plus .bak and .lock siblings.
func (e *Editor) Delete(_ context.Context, id string) error {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		return err
	}
	_ = os.Remove(path + ".bak")
	_ = os.Remove(path + ".lock")

	// Clean up the parent directory if empty.
	dir := filepath.Dir(path)
	if err := os.Remove(dir); err == nil {
		slog.Info("removed empty project dir", "dir", dir)
	}

	return nil
}

// RestoreBackup restores from the .bak sibling.
func (e *Editor) RestoreBackup(_ context.Context, id string) error {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		// The .jsonl may have been deleted; try the .bak directly.
		// Fall through using the ID under any project dir.
		return err
	}
	return editor.Restore(path)
}

// HasBackup reports whether a .bak file exists next to the session's JSONL.
func (e *Editor) HasBackup(id string) bool {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(path + ".bak")
	return err == nil
}
