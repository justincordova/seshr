package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Compile-time interface assertion.
var _ backend.LiveDetector = (*Detector)(nil)

// Detector implements backend.LiveDetector for OpenCode by correlating
// running OC processes to rows in the session table via their cwd.
//
// OpenCode has no sidecar file; the session-to-PID binding is inferred
// exclusively from (process.cwd == session.directory) AND session.
// time_updated within a 5-minute window of "now". When multiple sessions
// share the same cwd, every candidate is emitted as Ambiguous=true and
// the UI shows a non-confident live marker.
type Detector struct {
	store *Store
	now   func() time.Time

	cacheMu sync.Mutex
	cache   map[string]taskCacheEntry // session_id → last-seen task
}

// taskCacheEntry remembers the last time_updated and CurrentTask string
// observed for a session so we can skip the tool-part query when nothing
// changed between ticks.
type taskCacheEntry struct {
	TimeUpdated int64
	Task        string
}

// NewDetector constructs a Detector bound to store's read connection.
func NewDetector(store *Store) *Detector {
	return &Detector{
		store: store,
		now:   time.Now,
		cache: make(map[string]taskCacheEntry),
	}
}

// Kind returns SourceOpenCode.
func (d *Detector) Kind() session.SourceKind { return session.SourceOpenCode }

// DetectLive enumerates running OC processes and emits a LiveSession for
// every (process, session) pairing whose session was updated in the last
// 5 minutes.
//
// Flow:
//  1. Filter snap to OpenCode processes; de-dupe node/native launcher pairs.
//  2. For each surviving PID, query sessions in its cwd updated within 5m.
//  3. Emit one LiveSession per candidate; mark Ambiguous when PID maps to
//     multiple candidates.
//  4. Compute Status and CurrentTask for non-ambiguous emissions only.
func (d *Detector) DetectLive(ctx context.Context, snap backend.ProcessSnapshot) ([]backend.LiveSession, error) {
	procs := selectOpenCodeProcs(snap)
	if len(procs) == 0 {
		return nil, nil
	}

	now := d.now()
	cutoffMs := now.Add(-5 * time.Minute).UnixMilli()

	var out []backend.LiveSession
	for _, proc := range procs {
		cwd := proc.CWD
		if cwd == "" {
			continue
		}
		candidates, err := d.candidatesForCWD(ctx, cwd, cutoffMs)
		if err != nil {
			slog.Warn("opencode detector: query candidates failed",
				"pid", proc.PID, "cwd", cwd, "err", err)
			continue
		}
		if len(candidates) == 0 {
			continue
		}

		ambiguous := len(candidates) > 1
		children := childProcs(snap, proc.PID)

		for _, cand := range candidates {
			live := backend.LiveSession{
				SessionID:    cand.id,
				Kind:         session.SourceOpenCode,
				PID:          proc.PID,
				CWD:          cwd,
				Project:      filepath.Base(cwd),
				Status:       deriveStatus(cand, proc, children, now),
				LastActivity: time.UnixMilli(cand.timeUpdated),
				Ambiguous:    ambiguous,
			}
			if !ambiguous {
				live.CurrentTask = d.currentTask(ctx, cand.id, cand.timeUpdated, now)
			}
			out = append(out, live)
		}
	}
	return out, nil
}

// selectOpenCodeProcs filters snap to OC processes, dropping node launchers
// when a native child of the same agent exists (keep only the canonical agent
// process). This avoids double-counting the same session.
func selectOpenCodeProcs(snap backend.ProcessSnapshot) []backend.ProcInfo {
	// Collect PIDs whose argv[0..1] contains "opencode".
	ocPIDs := make(map[int]backend.ProcInfo)
	for pid, proc := range snap.ByPID {
		if !commandMentionsOpenCode(proc.Command) {
			continue
		}
		ocPIDs[pid] = proc
	}

	// For each PID, if its PPID is also an OC process, the PPID is a
	// launcher and should be dropped in favor of the child.
	dropped := make(map[int]struct{})
	for _, proc := range ocPIDs {
		if _, parentIsOC := ocPIDs[proc.PPID]; parentIsOC && proc.PPID != 0 {
			dropped[proc.PPID] = struct{}{}
		}
	}

	out := make([]backend.ProcInfo, 0, len(ocPIDs))
	for pid, proc := range ocPIDs {
		if _, skip := dropped[pid]; skip {
			continue
		}
		out = append(out, proc)
	}
	return out
}

// commandMentionsOpenCode mirrors isAgentCandidate's logic but scoped to OC.
// Used by selectOpenCodeProcs; takes argv[0..1] tokens into account so that
// launchers like `node /path/to/opencode` are detected via the second arg.
func commandMentionsOpenCode(command string) bool {
	tokens := strings.Fields(command)
	for i := 0; i < 2 && i < len(tokens); i++ {
		tok := tokens[i]
		if strings.Contains(tok, "opencode") {
			return true
		}
		if strings.Contains(filepath.Base(tok), "opencode") {
			return true
		}
	}
	return false
}

// candidateRow is a minimal session row used by the detector.
type candidateRow struct {
	id          string
	timeUpdated int64
}

// candidatesForCWD finds sessions whose directory matches cwd and were
// updated at or after cutoffMs.
//
// CWD canonicalization: filepath.Clean strips trailing slashes and normalizes
// "." / ".." components. If the first query returns nothing, the caller may
// be looking at a symlinked path that OC recorded differently; we retry with
// the EvalSymlinks result (best-effort; errors ignored).
func (d *Detector) candidatesForCWD(ctx context.Context, cwd string, cutoffMs int64) ([]candidateRow, error) {
	cleaned := filepath.Clean(cwd)
	rows, err := d.queryCandidates(ctx, cleaned, cutoffMs)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	// Retry with symlink-resolved path if different.
	resolved, symErr := filepath.EvalSymlinks(cleaned)
	if symErr == nil && resolved != "" && resolved != cleaned {
		return d.queryCandidates(ctx, resolved, cutoffMs)
	}
	return nil, nil
}

func (d *Detector) queryCandidates(ctx context.Context, cwd string, cutoffMs int64) ([]candidateRow, error) {
	const q = `
		SELECT id, time_updated
		FROM session
		WHERE directory = ?
		  AND time_archived IS NULL
		  AND time_updated > ?
		ORDER BY time_updated DESC
	`
	rows, err := d.store.conns.read.QueryContext(ctx, q, cwd, cutoffMs)
	if err != nil {
		return nil, fmt.Errorf("query candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []candidateRow
	for rows.Next() {
		var r candidateRow
		if err := rows.Scan(&r.id, &r.timeUpdated); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// deriveStatus implements the 3-signal derivation:
//  1. Session.time_updated within 30s → Working.
//  2. Agent process CPU > 1.0% → Working.
//  3. Any descendant process CPU > 5.0% → Working.
//  4. Otherwise Waiting.
//
// The DB signal takes precedence because it's the most reliable indicator
// that the agent is actively writing (vs. idle between user turns).
func deriveStatus(cand candidateRow, proc backend.ProcInfo, children []backend.ProcInfo, now time.Time) backend.Status {
	if now.Sub(time.UnixMilli(cand.timeUpdated)) < 30*time.Second {
		return backend.StatusWorking
	}
	if proc.CPU > 1.0 {
		return backend.StatusWorking
	}
	for _, c := range children {
		if c.CPU > 5.0 {
			return backend.StatusWorking
		}
	}
	return backend.StatusWaiting
}

// currentTask returns a human-readable description of the most recent tool
// invocation in the last 60 seconds, or "" if none. Caches per session_id
// keyed on time_updated so repeated ticks with no DB changes are free.
//
// Cache eviction: entries are never explicitly evicted. The cache is
// per-Detector-instance; when a session leaves the live set the entry just
// sits there with bounded memory (session IDs are ULID-sized). Acceptable
// for v1; a simple LRU is a post-v1 polish.
func (d *Detector) currentTask(ctx context.Context, sessionID string, timeUpdated int64, now time.Time) string {
	d.cacheMu.Lock()
	entry, ok := d.cache[sessionID]
	d.cacheMu.Unlock()
	if ok && entry.TimeUpdated == timeUpdated {
		return entry.Task
	}

	task := d.queryCurrentTask(ctx, sessionID, now)

	d.cacheMu.Lock()
	d.cache[sessionID] = taskCacheEntry{TimeUpdated: timeUpdated, Task: task}
	d.cacheMu.Unlock()
	return task
}

// queryCurrentTask runs the SQL and formats the result string. Cache miss
// path; isolated for testability.
func (d *Detector) queryCurrentTask(ctx context.Context, sessionID string, now time.Time) string {
	cutoffMs := now.Add(-60 * time.Second).UnixMilli()
	const q = `
		SELECT data FROM part
		WHERE session_id = ?
		  AND json_extract(data, '$.type') = 'tool'
		  AND time_created > ?
		  AND (json_extract(data, '$.state.status') = 'running'
		       OR json_extract(data, '$.state.status') = 'pending'
		       OR json_extract(data, '$.state.status') = 'completed')
		ORDER BY time_created DESC
		LIMIT 1
	`
	var raw []byte
	err := d.store.conns.read.QueryRowContext(ctx, q, sessionID, cutoffMs).Scan(&raw)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Debug("opencode detector: currentTask query failed",
				"session", sessionID, "err", err)
		}
		return ""
	}
	return formatToolPartForTask(raw)
}

// formatToolPartForTask renders the tool name + truncated first-arg as a
// single line, matching the Claude detector's 30-char budget.
//
// Fallback chain: full string → tool name only → "". Never panics on
// malformed JSON — the caller already tolerates "" as "no activity yet".
func formatToolPartForTask(raw []byte) string {
	var tp partTool
	if err := json.Unmarshal(raw, &tp); err != nil {
		return ""
	}
	if tp.Tool == "" {
		return ""
	}
	// Try to extract a first argument for context. OC tool input is a JSON
	// object; pick the first string-valued field.
	arg := firstStringArg(tp.State.Input)
	if arg == "" {
		return tp.Tool
	}
	full := tp.Tool + " " + arg
	return truncate(full, 30)
}

// firstStringArg peeks at a flat JSON object and returns the first string
// value encountered. Non-object or error → "".
func firstStringArg(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, v := range m {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// truncate returns s truncated to max runes, with an ellipsis when clipped.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

// childProcs returns the direct child ProcInfos of pid from snap.
func childProcs(snap backend.ProcessSnapshot, pid int) []backend.ProcInfo {
	out := make([]backend.ProcInfo, 0, len(snap.Children[pid]))
	for _, cpid := range snap.Children[pid] {
		if pi, ok := snap.ByPID[cpid]; ok {
			out = append(out, pi)
		}
	}
	return out
}
