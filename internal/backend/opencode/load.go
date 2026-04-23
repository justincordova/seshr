package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Load reads the full session and returns a parsed Session plus an opaque
// cursor pointing at the most-recent message. The cursor is Phase-10-shaped
// ahead of the incremental implementation so that a Load followed by
// LoadIncremental (shipped in Phase 10) works without a migration.
//
// OpenCode's schema has a parentID field in message.data JSON but the field
// does NOT form a linked list: assistant messages in a multi-step agent run
// all share the same user parent, and user messages have no parent. The
// session's conversation is simply every message ordered by
// (time_created, id). We preserve that ordering here.
//
// Branching (multiple assistants sharing a parent with divergent time lines)
// is handled by takeCurrentBranch — see branch.go.
func (s *Store) Load(ctx context.Context, id string) (*session.Session, backend.Cursor, error) {
	msgs, err := queryAllMessages(ctx, s.conns.read, id)
	if err != nil {
		return nil, backend.Cursor{}, err
	}
	if len(msgs) == 0 {
		return &session.Session{
			ID:     id,
			Source: session.SourceOpenCode,
		}, encodeCursor(cursorData{}), nil
	}

	chain := takeCurrentBranch(msgs)

	parts, err := queryPartsForMessages(ctx, s.conns.read, id, chainIDs(chain))
	if err != nil {
		return nil, backend.Cursor{}, err
	}

	decoded, err := decodeChain(chain, parts)
	if err != nil {
		return nil, backend.Cursor{}, err
	}

	sess := &session.Session{
		ID:                id,
		Source:            session.SourceOpenCode,
		Turns:             decoded.Turns,
		CompactBoundaries: decoded.Boundaries,
		TokenCount:        decoded.TotalTokens,
		ToolResultTokens:  decoded.ToolResultTokens,
	}
	last := chain[len(chain)-1]
	sess.ModifiedAt = msToTime(last.TimeCreated)

	cur := encodeCursor(cursorData{
		LastTimeCreated: last.TimeCreated,
		LastMessageID:   last.ID,
	})
	return sess, cur, nil
}

// queryAllMessages fetches every message for a session. Chain walking is
// done in memory — at ~40k messages across all sessions in the author's DB,
// a per-session query returns ≤ a few hundred rows and is sub-ms.
func queryAllMessages(ctx context.Context, db *sql.DB, sessionID string) ([]messageRow, error) {
	const q = `
		SELECT id, session_id, time_created, data
		FROM message
		WHERE session_id = ?
		ORDER BY time_created, id
	`
	rows, err := db.QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query messages for %s: %w", sessionID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []messageRow
	for rows.Next() {
		var m messageRow
		var raw []byte
		if err := rows.Scan(&m.ID, &m.SessionID, &m.TimeCreated, &raw); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		m.Data = json.RawMessage(raw)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter messages: %w", err)
	}
	return out, nil
}

// queryPartsForMessages fetches parts belonging to the given message IDs,
// ordered so decodeChain can consume them sequentially per-message. SQLite's
// parameter limit is 999 per statement; chunk defensively at 500 even though
// a single session typically stays under that.
func queryPartsForMessages(ctx context.Context, db *sql.DB, sessionID string, msgIDs []string) ([]partRow, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	const chunkSize = 500

	var out []partRow
	for start := 0; start < len(msgIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(msgIDs) {
			end = len(msgIDs)
		}
		chunk := msgIDs[start:end]

		placeholders := strings.Repeat("?,", len(chunk))
		placeholders = placeholders[:len(placeholders)-1] // trim trailing comma

		q := fmt.Sprintf(`
			SELECT id, message_id, session_id, time_created, data
			FROM part
			WHERE session_id = ?
			  AND message_id IN (%s)
			ORDER BY time_created, id
		`, placeholders)

		args := make([]any, 0, len(chunk)+1)
		args = append(args, sessionID)
		for _, id := range chunk {
			args = append(args, id)
		}

		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("query parts chunk: %w", err)
		}

		for rows.Next() {
			var p partRow
			var raw []byte
			if err := rows.Scan(&p.ID, &p.MessageID, &p.SessionID, &p.TimeCreated, &raw); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan part row: %w", err)
			}
			p.Data = json.RawMessage(raw)
			out = append(out, p)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iter parts: %w", err)
		}
		_ = rows.Close()
	}
	return out, nil
}

// chainIDs extracts the message IDs from a chain in order.
func chainIDs(chain []messageRow) []string {
	ids := make([]string, len(chain))
	for i, m := range chain {
		ids[i] = m.ID
	}
	return ids
}
