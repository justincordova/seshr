package claude

import (
	"context"
	"fmt"
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
//
// The per-session flock serializes seshr's own destructive operations
// (Prune vs Delete vs Restore). Claude Code does NOT participate in the
// flock, so two additional guards protect against the live agent:
//   - selection timestamps (when provided) are verified against the fresh
//     parse, so a transcript rewrite between the UI's load and this prune
//     cannot shift indices onto the wrong turns;
//   - the file is re-stat'ed immediately before the atomic replace, so
//     records appended mid-prune abort the operation instead of being
//     silently truncated away.
func (e *Editor) Prune(ctx context.Context, id string, sel backend.Selection) (backend.PruneResult, error) {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return backend.PruneResult{}, err
	}

	lock, err := editor.TryLock(path)
	if err != nil {
		return backend.PruneResult{}, err
	}
	defer func() { _ = lock.Release() }()

	preInfo, err := os.Stat(path)
	if err != nil {
		return backend.PruneResult{}, fmt.Errorf("stat %s: %w", path, err)
	}

	p := NewClaude()
	sess, err := p.Parse(ctx, path)
	if err != nil {
		return backend.PruneResult{}, err
	}

	// Verify the selection still points at the turns the user saw. Indices
	// are positional; if the agent rewrote/compacted the transcript (or the
	// UI's index space drifted), timestamps won't line up.
	if err := verifySelection(sess, sel); err != nil {
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

	// An empty expansion (e.g. the selection contained only system/summary
	// turns) is a no-op: rewriting would change nothing, but CreateBackup
	// would still overwrite the .bak with current content.
	if len(expanded.Turns) == 0 {
		return backend.PruneResult{}, nil
	}

	// pruneWithoutLock does the same work as PruneSession but without
	// re-acquiring the flock we already hold.
	if err := pruneWithoutLock(ctx, sess, expanded, preInfo); err != nil {
		return backend.PruneResult{}, err
	}

	return backend.PruneResult{}, nil
}

// verifySelection checks that each selected turn's timestamp (when the
// caller provided them) matches the freshly parsed session.
func verifySelection(sess *session.Session, sel backend.Selection) error {
	if len(sel.TurnTimestamps) == 0 {
		return nil
	}
	if len(sel.TurnTimestamps) != len(sel.TurnIndices) {
		return fmt.Errorf("selection has %d indices but %d timestamps", len(sel.TurnIndices), len(sel.TurnTimestamps))
	}
	for i, idx := range sel.TurnIndices {
		if idx < 0 || idx >= len(sess.Turns) {
			return backend.ErrSelectionStale
		}
		want := sel.TurnTimestamps[i]
		if want.IsZero() {
			continue
		}
		if !sess.Turns[idx].Timestamp.Equal(want) {
			return backend.ErrSelectionStale
		}
	}
	return nil
}

// pruneWithoutLock backs Prune above. Callers MUST already hold the
// per-session flock; this is unexported to keep the lock invariant local.
// preInfo is the source file's stat from before the parse — used to detect
// a live agent writing mid-prune.
func pruneWithoutLock(ctx context.Context, sess *session.Session, expanded editor.Selection, preInfo os.FileInfo) error {
	path := sess.Path

	if err := editor.CreateBackup(path); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := editor.Prune(sess, expanded, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	// Validate the pruned output BEFORE it replaces the live file: it must
	// parse cleanly and contain exactly the expected number of turns. This is
	// the last line of defense against a selection/pairing bug corrupting a
	// session (the .bak would still exist, but the user shouldn't need it).
	after, err := NewClaude().Parse(ctx, tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("pruned output failed validation: %w", err)
	}
	if want := len(sess.Turns) - len(expanded.Turns); len(after.Turns) != want {
		_ = os.Remove(tmp)
		return fmt.Errorf("pruned output has %d turns, want %d: aborting before replace", len(after.Turns), want)
	}

	// Last-moment write detection: the flock only excludes other seshr
	// processes — a live Claude Code agent can append while we rewrite.
	// Anything it wrote after our parse would be destroyed by the replace,
	// so abort instead and let the user retry.
	if curInfo, statErr := os.Stat(path); statErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("re-stat before replace: %w", statErr)
	} else if curInfo.Size() != preInfo.Size() || !curInfo.ModTime().Equal(preInfo.ModTime()) {
		_ = os.Remove(tmp)
		return fmt.Errorf("session file changed while pruning (live agent wrote to it); retry: %w", backend.ErrSelectionStale)
	}

	if err := editor.AtomicReplace(tmp, path); err != nil {
		return err
	}

	slog.Info("pruned session",
		"path", path,
		"removed_turns", len(expanded.Turns),
	)
	return nil
}

// Delete removes the JSONL file but PRESERVES the .bak so a user who
// reflexively deletes a session can still restore it. The .bak is cleaned
// up only when the parent project dir is also removed (i.e. there is
// nothing left to clean up beyond it).
//
// Holds the per-session flock for the duration so a concurrent Prune
// cannot have its read race against this delete. Crucially, we do NOT
// remove the .lock file itself — a flock is bound to the file inode, and
// unlinking the directory entry mid-prune leaves the prune holding a
// flock on an orphaned inode while a fresh TryLock would create a new
// file with a different inode and "succeed" against it.
func (e *Editor) Delete(_ context.Context, id string) error {
	path, err := e.store.transcriptPath(id)
	if err != nil {
		return err
	}

	lock, err := editor.TryLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()

	// Take a snapshot only when no .bak exists yet. A pre-existing .bak is
	// almost certainly from a recent prune — overwriting it here would
	// silently turn a Prune→Delete→Restore round-trip into "restore
	// recovers the just-pre-delete state" instead of "restore recovers
	// what existed before the user's last destructive edit".
	if _, statErr := os.Stat(path + ".bak"); os.IsNotExist(statErr) {
		if err := editor.CreateBackup(path); err != nil {
			slog.Warn("delete: pre-removal backup failed", "id", id, "err", err)
		}
	}

	if err := os.Remove(path); err != nil {
		return err
	}

	// Clean up the parent directory if empty. We deliberately leave behind
	// the .lock and .bak siblings; RestoreBackup needs the .bak, and the
	// .lock costs nothing on disk while preventing the inode-orphan race
	// described above. os.Remove on a non-empty dir errors silently.
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
