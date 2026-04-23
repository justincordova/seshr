package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// maxIncrementalRows caps per-tick work. 1000 rows is ~8x the peak burst
// we've ever seen between fast ticks (2s). A pathological long-running agent
// loop could exceed this; we re-enter the query on the next tick and catch
// up, so the cap degrades gracefully.
const maxIncrementalRows = 1000

// LoadIncremental reads messages (and their parts) appended since the last
// cursor, decodes them via decodeChain, and returns the new turns plus an
// updated cursor.
//
// Strategy: OpenCode sessions are chronologically ordered by
// (time_created, id). LoadIncremental queries messages with that key
// strictly greater than the cursor's, caps at maxIncrementalRows, pulls
// their parts in one chunked IN-list query, and decodes.
//
// Edge case: Regen branching. If the user regens a PAST user message while
// the cockpit is open, OC emits a new assistant sibling with time_created
// > cursor. That new message will be included (as expected) but the older
// sibling is stale. Since we rely on the ordered stream, the stale sibling
// simply stays rendered — a cosmetic inconsistency. The fast-tick chain
// reload on cursor exhaustion handles persistent-drift cases.
//
// Cap detection: if the query fills maxIncrementalRows we return with the
// cursor advanced only through the returned rows; the next tick resumes
// from there.
func (s *Store) LoadIncremental(ctx context.Context, id string, cur backend.Cursor) ([]session.Turn, backend.Cursor, error) {
	cd, err := decodeCursor(cur)
	if err != nil {
		return nil, cur, err
	}

	if cd.LastMessageID == "" {
		// Cold cursor: fall through to a full Load and return all turns.
		// Callers expect the cursor round-trip to work from the Load result.
		sess, newCur, err := s.Load(ctx, id)
		if err != nil {
			return nil, cur, err
		}
		return sess.Turns, newCur, nil
	}

	newMsgs, err := queryMessagesAfter(ctx, s.conns.read, id, cd.LastTimeCreated, cd.LastMessageID, maxIncrementalRows)
	if err != nil {
		return nil, cur, err
	}
	if len(newMsgs) == 0 {
		return nil, cur, nil
	}

	parts, err := queryPartsForMessages(ctx, s.conns.read, id, chainIDs(newMsgs))
	if err != nil {
		return nil, cur, err
	}

	// decodeChain assumes caller-provided chronological order; our SQL gives
	// that to us. We do NOT re-run takeCurrentBranch here: the messages are
	// strictly newer than cursor, and any branching happens inside the
	// window of returned rows. For v1 we accept that a mid-window regen is
	// rendered verbatim; the session reconciler (fast-tick → LoadIncremental
	// → chain rebuild if empty) is the correction path.
	decoded, err := decodeChain(newMsgs, parts)
	if err != nil {
		return nil, cur, err
	}

	last := newMsgs[len(newMsgs)-1]
	newCur := encodeCursor(cursorData{
		LastTimeCreated: last.TimeCreated,
		LastMessageID:   last.ID,
	})
	return decoded.Turns, newCur, nil
}

// queryMessagesAfter fetches messages whose (time_created, id) is strictly
// greater than the given cursor, in ascending order, capped at limit.
//
// The strictly-greater predicate decomposes as:
//
//	time_created > ?
//	OR (time_created = ? AND id > ?)
//
// Message IDs in OC are ULID-ish (msg_<random>). ULID comparison is lexical
// by design, so `id > ?` is a valid tiebreak within the same ms bucket.
func queryMessagesAfter(ctx context.Context, db *sql.DB, sessionID string, sinceMs int64, sinceID string, limit int) ([]messageRow, error) {
	const q = `
		SELECT id, session_id, time_created, data
		FROM message
		WHERE session_id = ?
		  AND (time_created > ? OR (time_created = ? AND id > ?))
		ORDER BY time_created, id
		LIMIT ?
	`
	rows, err := db.QueryContext(ctx, q, sessionID, sinceMs, sinceMs, sinceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query messages after cursor: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []messageRow
	for rows.Next() {
		var m messageRow
		var raw []byte
		if err := rows.Scan(&m.ID, &m.SessionID, &m.TimeCreated, &raw); err != nil {
			return nil, fmt.Errorf("scan message after cursor: %w", err)
		}
		m.Data = json.RawMessage(raw)
		out = append(out, m)
	}
	return out, rows.Err()
}

// LoadRange returns turns [from, to) from the session's current chain.
//
// Implementation: re-load the full chain and slice. The plan suggests a
// caching layer; for v1 we take the hit. A single session's chain is
// bounded by a few hundred messages at most in practice (OC auto-compacts
// long runs); re-querying per scroll-back is O(rows-in-session), well under
// 100ms even on the author's largest fixture.
func (s *Store) LoadRange(ctx context.Context, id string, fromIdx, toIdx int) ([]session.Turn, error) {
	if fromIdx < 0 {
		return nil, fmt.Errorf("invalid range: from=%d", fromIdx)
	}
	if toIdx < fromIdx {
		return nil, fmt.Errorf("invalid range: to=%d < from=%d", toIdx, fromIdx)
	}

	sess, _, err := s.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	n := len(sess.Turns)
	if fromIdx >= n {
		return nil, nil
	}
	if toIdx > n {
		toIdx = n
	}
	// Defensive copy so callers can't mutate the cached slice.
	out := make([]session.Turn, toIdx-fromIdx)
	copy(out, sess.Turns[fromIdx:toIdx])
	return out, nil
}
