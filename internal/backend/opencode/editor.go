package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	sqlite3lib "modernc.org/sqlite"
	sqlite3errs "modernc.org/sqlite/lib"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Compile-time interface assertion.
var _ backend.SessionEditor = (*Editor)(nil)

// ErrSessionBusy is returned by Prune/Delete when the write transaction
// could not acquire within the busy_timeout. Callers (TUI) surface this as
// "session busy, try again" without retrying automatically — users who see
// the message may want to stop the agent first.
var ErrSessionBusy = errors.New("opencode session is busy; try again")

// ErrConcurrentPrune is returned when another seshr process holds the
// per-session backup lock. Rare in practice (user running two seshr at once)
// but produces a stable error message for the caller.
var ErrConcurrentPrune = errors.New("another seshr is pruning this session")

// backupRetention is the max number of backup files kept per session. Older
// files are deleted after each new backup write.
const backupRetention = 5

// deleteChunkSize bounds the number of IN-list params in a DELETE statement.
// SQLite's compiled-default limit is 999; 500 leaves headroom.
const deleteChunkSize = 500

// Editor implements backend.SessionEditor for OpenCode.
type Editor struct {
	store     *Store
	backupDir string
	now       func() time.Time
}

// NewEditor constructs an Editor bound to store. backupDir is the root
// directory ~/.seshr/backups/opencode — per-session subdirs are created on
// demand. Pass an empty string to disable backups entirely (prune/delete
// will still work but no restore is possible).
func NewEditor(store *Store, backupDir string) *Editor {
	return &Editor{
		store:     store,
		backupDir: backupDir,
		now:       time.Now,
	}
}

// Kind returns SourceOpenCode.
func (e *Editor) Kind() session.SourceKind { return session.SourceOpenCode }

// ── Prune ────────────────────────────────────────────────────────────────

// Prune removes the selected turns from the session's current chain. For
// OpenCode, a "turn" maps to a single message (and all its parts). Running
// and pending tool parts are intentionally preserved to avoid yanking the
// ground out from under the live agent mid-call.
//
// Flow:
//  1. Resolve turn indices → (message IDs, part IDs, skipped running count).
//  2. Read the rows we're about to destroy for the backup payload.
//  3. Acquire the per-session lock, write the backup, apply retention.
//  4. Open a write tx with BEGIN IMMEDIATE + foreign_keys=on.
//  5. Chunked DELETE on part then message (FK-safe order).
//  6. COMMIT; on any error ROLLBACK and surface.
func (e *Editor) Prune(ctx context.Context, id string, sel backend.Selection) (backend.PruneResult, error) {
	if len(sel.TurnIndices) == 0 {
		return backend.PruneResult{}, nil
	}

	resolved, err := e.resolveSelection(ctx, id, sel.TurnIndices)
	if err != nil {
		return backend.PruneResult{}, fmt.Errorf("resolve selection: %w", err)
	}
	if len(resolved.MsgIDs) == 0 {
		return backend.PruneResult{SkippedRunningTools: resolved.SkippedRunningTools}, nil
	}

	msgs, parts, err := e.readRowsForBackup(ctx, id, resolved.MsgIDs, resolved.PartIDs)
	if err != nil {
		return backend.PruneResult{}, fmt.Errorf("read backup rows: %w", err)
	}

	if err := e.withSessionLock(id, func() error {
		return e.writeAndRetainBackup(id, "prune", msgs, parts)
	}); err != nil {
		return backend.PruneResult{}, err
	}

	if err := e.execPruneTx(ctx, resolved); err != nil {
		return backend.PruneResult{}, err
	}

	return backend.PruneResult{SkippedRunningTools: resolved.SkippedRunningTools}, nil
}

// resolvedSelection is the output of resolveSelection.
type resolvedSelection struct {
	MsgIDs              []string
	PartIDs             []string
	SkippedRunningTools int
}

// resolveSelection walks the current chain, maps the requested turn indices
// to OC message IDs, and collects all PRUNABLE part IDs belonging to them.
// Running/pending tool parts are excluded from the delete set; we count them
// so the caller can toast "N tool calls skipped".
func (e *Editor) resolveSelection(ctx context.Context, id string, turnIdx []int) (resolvedSelection, error) {
	msgs, err := queryAllMessages(ctx, e.store.conns.read, id)
	if err != nil {
		return resolvedSelection{}, err
	}
	chain := takeCurrentBranch(msgs)
	if len(chain) == 0 {
		return resolvedSelection{}, fmt.Errorf("session %s has no messages", id)
	}

	// Deduplicate + validate indices.
	wanted := make(map[int]struct{}, len(turnIdx))
	for _, i := range turnIdx {
		if i < 0 || i >= len(chain) {
			return resolvedSelection{}, fmt.Errorf("turn index out of range: %d (have %d turns)", i, len(chain))
		}
		wanted[i] = struct{}{}
	}

	msgIDs := make([]string, 0, len(wanted))
	for i, m := range chain {
		if _, ok := wanted[i]; ok {
			msgIDs = append(msgIDs, m.ID)
		}
	}

	// Pull parts for the targeted messages and partition into
	// deletable vs skipped-because-running.
	parts, err := queryPartsForMessages(ctx, e.store.conns.read, id, msgIDs)
	if err != nil {
		return resolvedSelection{}, err
	}

	partIDs := make([]string, 0, len(parts))
	skipped := 0
	for _, p := range parts {
		if partIsLiveTool(p.Data) {
			skipped++
			continue
		}
		partIDs = append(partIDs, p.ID)
	}

	// If every part of a message is preserved (live tool), the message itself
	// should NOT be deleted either — otherwise we'd orphan the part via
	// CASCADE. Drop such messages from MsgIDs.
	//
	// Edge case defense: count deletable parts per message; messages whose
	// parts are entirely skipped get re-examined.
	msgIDs = filterMessagesWithAllPartsLive(msgIDs, parts)

	return resolvedSelection{
		MsgIDs:              msgIDs,
		PartIDs:             partIDs,
		SkippedRunningTools: skipped,
	}, nil
}

// partIsLiveTool returns true for tool parts whose state.status is running
// or pending. Running tools have output still being generated; deleting the
// part would stream into nothing and surface as a future FK constraint
// violation when the completion write lands.
func partIsLiveTool(raw json.RawMessage) bool {
	var head struct {
		Type  string `json:"type"`
		State struct {
			Status string `json:"status"`
		} `json:"state"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return false
	}
	if head.Type != "tool" {
		return false
	}
	return head.State.Status == "running" || head.State.Status == "pending"
}

// filterMessagesWithAllPartsLive returns only those msgIDs that have at
// least one non-live part (i.e., at least one row we'd actually delete).
// A message whose every part is live gets preserved whole — deleting the
// message via CASCADE would remove its live parts too.
func filterMessagesWithAllPartsLive(msgIDs []string, parts []partRow) []string {
	wanted := make(map[string]bool, len(msgIDs))
	for _, id := range msgIDs {
		wanted[id] = false // false = no deletable part seen yet
	}
	for _, p := range parts {
		if _, ok := wanted[p.MessageID]; !ok {
			continue
		}
		if !partIsLiveTool(p.Data) {
			wanted[p.MessageID] = true
		}
	}
	out := msgIDs[:0:len(msgIDs)]
	for _, id := range msgIDs {
		if wanted[id] {
			out = append(out, id)
		}
	}
	return out
}

// execPruneTx runs the chunked DELETE in a BEGIN IMMEDIATE transaction.
// Part rows are deleted first (even though the FK cascades, doing the
// explicit DELETE gives us a precise error count and keeps the tx small
// enough that a busy mid-statement doesn't leave partial state).
func (e *Editor) execPruneTx(ctx context.Context, r resolvedSelection) error {
	conn, err := e.writeConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if err := beginImmediate(ctx, conn); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if err := chunkedDelete(ctx, conn, "part", r.PartIDs); err != nil {
		return fmt.Errorf("delete parts: %w", err)
	}
	if err := chunkedDelete(ctx, conn, "message", r.MsgIDs); err != nil {
		return fmt.Errorf("delete messages: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit prune: %w", err)
	}
	committed = true
	return nil
}

// ── Delete (whole session) ────────────────────────────────────────────────

// Delete removes the entire session from the DB. A backup is written first
// so Restore is available. The FK cascade on session → message → part
// handles the child rows in a single statement.
func (e *Editor) Delete(ctx context.Context, id string) error {
	msgs, parts, err := e.readWholeSessionForBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("read session for delete backup: %w", err)
	}

	if err := e.withSessionLock(id, func() error {
		return e.writeAndRetainBackup(id, "delete", msgs, parts)
	}); err != nil {
		return err
	}

	conn, err := e.writeConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if err := beginImmediate(ctx, conn); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if _, err := conn.ExecContext(ctx, "DELETE FROM session WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete session row: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	committed = true
	return nil
}

// ── RestoreBackup ─────────────────────────────────────────────────────────

// RestoreBackup re-inserts the most recent backup for the given session.
// Idempotent via INSERT OR IGNORE — restoring twice is a no-op. If the
// backup was a delete-mode one, the session row is re-inserted too.
func (e *Editor) RestoreBackup(ctx context.Context, id string) error {
	dir := e.sessionBackupDir(id)
	path, err := latestBackupPath(dir)
	if err != nil {
		return err
	}

	payload, err := readBackupFile(path)
	if err != nil {
		return fmt.Errorf("read backup %s: %w", path, err)
	}

	if err := e.withSessionLock(id, func() error {
		return e.execRestoreTx(ctx, payload)
	}); err != nil {
		return err
	}
	slog.Info("opencode restore complete", "session", id, "mode", payload.Mode, "file", path)
	return nil
}

func (e *Editor) execRestoreTx(ctx context.Context, payload backupPayload) error {
	conn, err := e.writeConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if err := beginImmediate(ctx, conn); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	// Delete-mode backups snapshotted the session row; re-insert via the
	// SessionRow payload so the parent exists before we insert children.
	if payload.Mode == "delete" && payload.Session != nil {
		if err := insertSessionRow(ctx, conn, payload.Session); err != nil {
			return err
		}
	}

	for _, m := range payload.Messages {
		if _, err := conn.ExecContext(ctx,
			`INSERT OR IGNORE INTO message (id, session_id, time_created, time_updated, data)
			 VALUES (?, ?, ?, ?, ?)`,
			m.ID, m.SessionID, m.TimeCreated, m.TimeCreated, []byte(m.Data),
		); err != nil {
			return fmt.Errorf("restore message %s: %w", m.ID, err)
		}
	}
	for _, p := range payload.Parts {
		if _, err := conn.ExecContext(ctx,
			`INSERT OR IGNORE INTO part (id, message_id, session_id, time_created, time_updated, data)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, p.MessageID, p.SessionID, p.TimeCreated, p.TimeCreated, []byte(p.Data),
		); err != nil {
			return fmt.Errorf("restore part %s: %w", p.ID, err)
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit restore: %w", err)
	}
	committed = true
	return nil
}

// HasBackup returns true when the session's backup directory contains at
// least one JSON file.
func (e *Editor) HasBackup(id string) bool {
	if e.backupDir == "" {
		return false
	}
	entries, err := os.ReadDir(e.sessionBackupDir(id))
	if err != nil {
		return false
	}
	for _, en := range entries {
		if !en.IsDir() && strings.HasSuffix(en.Name(), ".json") {
			return true
		}
	}
	return false
}

// ── Backup payload + IO ──────────────────────────────────────────────────

// backupPayload is the on-disk shape. Mode distinguishes a prune backup
// (subset of parts + owning messages) from a delete backup (full session
// row + all messages + all parts).
type backupPayload struct {
	Version   int             `json:"version"`
	Source    string          `json:"source"`
	SessionID string          `json:"session_id"`
	Mode      string          `json:"mode"` // "prune" | "delete"
	PrunedAt  string          `json:"pruned_at"`
	Session   *sessionRowBak  `json:"session,omitempty"` // populated for delete mode
	Messages  []messageRowBak `json:"messages"`
	Parts     []partRowBak    `json:"parts"`
}

type sessionRowBak struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Slug         string `json:"slug"`
	Directory    string `json:"directory"`
	Title        string `json:"title"`
	Version      string `json:"version"`
	TimeCreated  int64  `json:"time_created"`
	TimeUpdated  int64  `json:"time_updated"`
	TimeArchived *int64 `json:"time_archived,omitempty"`
}

type messageRowBak struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	TimeCreated int64           `json:"time_created"`
	Data        json.RawMessage `json:"data"`
}

type partRowBak struct {
	ID          string          `json:"id"`
	MessageID   string          `json:"message_id"`
	SessionID   string          `json:"session_id"`
	TimeCreated int64           `json:"time_created"`
	Data        json.RawMessage `json:"data"`
}

// writeAndRetainBackup writes the backup file and prunes older backups past
// the retention limit. Caller must hold the per-session flock.
func (e *Editor) writeAndRetainBackup(id, mode string, msgs []messageRowBak, parts []partRowBak) error {
	if e.backupDir == "" {
		return nil // backups disabled
	}
	dir := e.sessionBackupDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir backup dir: %w", err)
	}

	ts := e.now().Format("20060102-150405")
	name := ts + ".json"
	if mode == "delete" {
		name = ts + "-delete.json"
	}
	path := filepath.Join(dir, name)

	payload := backupPayload{
		Version:   1,
		Source:    "opencode",
		SessionID: id,
		Mode:      mode,
		PrunedAt:  e.now().UTC().Format(time.RFC3339),
		Messages:  msgs,
		Parts:     parts,
	}

	// For delete mode, snapshot the session row too.
	if mode == "delete" {
		row, err := e.readSessionRowBak(context.Background(), id)
		if err != nil {
			return fmt.Errorf("snapshot session row: %w", err)
		}
		payload.Session = row
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal backup: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write backup %s: %w", path, err)
	}

	if err := applyRetention(dir, backupRetention); err != nil {
		slog.Warn("backup retention failed", "dir", dir, "err", err)
	}
	return nil
}

// sessionBackupDir returns the per-session backup directory under backupDir.
func (e *Editor) sessionBackupDir(id string) string {
	return filepath.Join(e.backupDir, id)
}

// applyRetention deletes all but the `keep` most-recent *.json files in dir.
// Sorted by filename (timestamp-prefixed) so lexical sort == chronological.
func applyRetention(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, en := range entries {
		if !en.IsDir() && strings.HasSuffix(en.Name(), ".json") {
			names = append(names, en.Name())
		}
	}
	if len(names) <= keep {
		return nil
	}
	sort.Strings(names) // ascending == oldest first
	var errs []error
	for _, n := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// latestBackupPath returns the lex-greatest (== newest by timestamp) *.json
// file in dir. Returns an empty-path error if none found.
func latestBackupPath(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, en := range entries {
		if !en.IsDir() && strings.HasSuffix(en.Name(), ".json") {
			names = append(names, en.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no backups in %s", dir)
	}
	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1]), nil
}

// readBackupFile decodes the backup JSON at path.
func readBackupFile(path string) (backupPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return backupPayload{}, err
	}
	var p backupPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return backupPayload{}, err
	}
	return p, nil
}

// readRowsForBackup queries the rows about to be deleted so we have the
// payload for the backup file. Parts query uses the full PartIDs list (not
// MsgIDs) because we want exactly the parts being deleted, not all parts of
// the targeted messages (some of those are live-tool and being preserved).
func (e *Editor) readRowsForBackup(ctx context.Context, sessionID string, msgIDs, partIDs []string) ([]messageRowBak, []partRowBak, error) {
	msgs, err := queryMessageRowsByID(ctx, e.store.conns.read, sessionID, msgIDs)
	if err != nil {
		return nil, nil, err
	}
	parts, err := queryPartRowsByID(ctx, e.store.conns.read, sessionID, partIDs)
	if err != nil {
		return nil, nil, err
	}
	return msgs, parts, nil
}

// readWholeSessionForBackup fetches EVERY message + part in the session for
// a delete-mode backup. Used by Delete.
func (e *Editor) readWholeSessionForBackup(ctx context.Context, sessionID string) ([]messageRowBak, []partRowBak, error) {
	rawMsgs, err := queryAllMessages(ctx, e.store.conns.read, sessionID)
	if err != nil {
		return nil, nil, err
	}
	msgs := make([]messageRowBak, 0, len(rawMsgs))
	for _, m := range rawMsgs {
		msgs = append(msgs, messageRowBak(m))
	}
	ids := make([]string, len(rawMsgs))
	for i, m := range rawMsgs {
		ids[i] = m.ID
	}
	rawParts, err := queryPartsForMessages(ctx, e.store.conns.read, sessionID, ids)
	if err != nil {
		return nil, nil, err
	}
	parts := make([]partRowBak, 0, len(rawParts))
	for _, p := range rawParts {
		parts = append(parts, partRowBak(p))
	}
	return msgs, parts, nil
}

// readSessionRowBak reads a single session row for delete backup payload.
func (e *Editor) readSessionRowBak(ctx context.Context, id string) (*sessionRowBak, error) {
	const q = `
		SELECT id, project_id, slug, directory, title, version,
		       time_created, time_updated, time_archived
		FROM session WHERE id = ?
	`
	var r sessionRowBak
	var archived sql.NullInt64
	err := e.store.conns.read.QueryRowContext(ctx, q, id).Scan(
		&r.ID, &r.ProjectID, &r.Slug, &r.Directory, &r.Title, &r.Version,
		&r.TimeCreated, &r.TimeUpdated, &archived,
	)
	if err != nil {
		return nil, err
	}
	if archived.Valid {
		v := archived.Int64
		r.TimeArchived = &v
	}
	return &r, nil
}

// insertSessionRow reinserts a session row on delete-mode restore.
func insertSessionRow(ctx context.Context, conn *sql.Conn, r *sessionRowBak) error {
	var archived any
	if r.TimeArchived != nil {
		archived = *r.TimeArchived
	}
	_, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO session
			(id, project_id, slug, directory, title, version,
			 time_created, time_updated, time_archived)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.ProjectID, r.Slug, r.Directory, r.Title, r.Version,
		r.TimeCreated, r.TimeUpdated, archived,
	)
	return err
}

// queryMessageRowsByID fetches only the specified messages for a session.
func queryMessageRowsByID(ctx context.Context, db *sql.DB, sessionID string, ids []string) ([]messageRowBak, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []messageRowBak
	for start := 0; start < len(ids); start += deleteChunkSize {
		end := start + deleteChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.Repeat("?,", end-start)
		placeholders = placeholders[:len(placeholders)-1]

		args := make([]any, 0, end-start+1)
		args = append(args, sessionID)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		q := fmt.Sprintf(`
			SELECT id, session_id, time_created, data
			FROM message WHERE session_id = ? AND id IN (%s)
		`, placeholders)
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r messageRowBak
			var raw []byte
			if err := rows.Scan(&r.ID, &r.SessionID, &r.TimeCreated, &raw); err != nil {
				_ = rows.Close()
				return nil, err
			}
			r.Data = json.RawMessage(raw)
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return out, nil
}

// queryPartRowsByID mirrors queryMessageRowsByID for parts.
func queryPartRowsByID(ctx context.Context, db *sql.DB, sessionID string, ids []string) ([]partRowBak, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []partRowBak
	for start := 0; start < len(ids); start += deleteChunkSize {
		end := start + deleteChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := strings.Repeat("?,", end-start)
		placeholders = placeholders[:len(placeholders)-1]

		args := make([]any, 0, end-start+1)
		args = append(args, sessionID)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		q := fmt.Sprintf(`
			SELECT id, message_id, session_id, time_created, data
			FROM part WHERE session_id = ? AND id IN (%s)
		`, placeholders)
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r partRowBak
			var raw []byte
			if err := rows.Scan(&r.ID, &r.MessageID, &r.SessionID, &r.TimeCreated, &raw); err != nil {
				_ = rows.Close()
				return nil, err
			}
			r.Data = json.RawMessage(raw)
			out = append(out, r)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return out, nil
}

// ── Locking + write connection ───────────────────────────────────────────

// withSessionLock acquires an exclusive flock on <sessionBackupDir>/.lock
// for the duration of fn. Returns ErrConcurrentPrune when another process
// holds it. Creates the directory if missing.
func (e *Editor) withSessionLock(id string, fn func() error) error {
	if e.backupDir == "" {
		// No backup dir => no lock needed. Used only by tests that disable
		// backups; production always has a backupDir.
		return fn()
	}
	dir := e.sessionBackupDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir lock dir: %w", err)
	}
	l := flock.New(filepath.Join(dir, ".lock"))
	locked, err := l.TryLock()
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		return ErrConcurrentPrune
	}
	defer func() { _ = l.Unlock() }()
	return fn()
}

// writeConn opens a dedicated connection from the lazily-initialized write
// DB, so BEGIN IMMEDIATE sticks to this exact connection.
func (e *Editor) writeConn(ctx context.Context) (*sql.Conn, error) {
	wdb, err := e.store.conns.openWrite()
	if err != nil {
		return nil, err
	}
	return wdb.Conn(ctx)
}

// beginImmediate starts an IMMEDIATE transaction, translating SQLITE_BUSY to
// ErrSessionBusy so the caller can show a concise toast.
func beginImmediate(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		if isSQLiteBusy(err) {
			return ErrSessionBusy
		}
		return fmt.Errorf("begin immediate: %w", err)
	}
	return nil
}

// isSQLiteBusy unwraps the SQLite-specific error code.
func isSQLiteBusy(err error) bool {
	var se *sqlite3lib.Error
	if errors.As(err, &se) {
		return se.Code() == sqlite3errs.SQLITE_BUSY
	}
	return false
}

// chunkedDelete issues DELETE ... WHERE id IN (...) in chunks of
// deleteChunkSize. Empty input is a no-op.
func chunkedDelete(ctx context.Context, conn *sql.Conn, table string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	for start := 0; start < len(ids); start += deleteChunkSize {
		end := start + deleteChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id)
		}
		q := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", table, placeholders)
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("%s delete chunk: %w", table, err)
		}
	}
	return nil
}
