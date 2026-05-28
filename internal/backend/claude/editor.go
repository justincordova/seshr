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

// Delete removes the JSONL file (and its .lock sibling) but PRESERVES the
// .bak so a user who reflexively deletes a session can still restore it.
// The .bak is cleaned up only when the parent project dir is also removed
// (i.e. there is nothing left to clean up beyond it).
func (e *Editor) Delete(_ context.Context, id string) error {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return err
	}

	// Take a defensive backup before removing the transcript. CreateBackup
	// silently overwrites an existing .bak — that's the intended behavior
	// here (the freshest snapshot wins).
	if err := editor.CreateBackup(path); err != nil {
		slog.Warn("delete: pre-removal backup failed", "id", id, "err", err)
	}

	if err := os.Remove(path); err != nil {
		return err
	}
	_ = os.Remove(path + ".lock")

	// Clean up the parent directory if empty. We do NOT remove the .bak —
	// RestoreBackup uses Store.backupPath to locate it even when the .jsonl
	// is gone. The directory removal is best-effort: if a .bak (or anything
	// else) still lives there, Go's os.Remove on a non-empty dir errors and
	// we leave it in place.
	dir := filepath.Dir(path)
	if err := os.Remove(dir); err == nil {
		slog.Info("removed empty project dir", "dir", dir)
	}

	return nil
}

// RestoreBackup restores from the .bak sibling. Works even after Delete,
// since backupPath does not require the .jsonl to be present.
func (e *Editor) RestoreBackup(_ context.Context, id string) error {
	bak, err := e.store.backupPath(id)
	if err != nil {
		return err
	}
	// editor.Restore copies <path>.bak -> <path>; derive the target path
	// by stripping the .bak suffix.
	jsonlPath := bak[:len(bak)-len(".bak")]
	return editor.Restore(jsonlPath)
}

// HasBackup reports whether a .bak file exists for this session, even when
// the original .jsonl has been deleted.
func (e *Editor) HasBackup(id string) bool {
	_, err := e.store.backupPath(id)
	return err == nil
}
