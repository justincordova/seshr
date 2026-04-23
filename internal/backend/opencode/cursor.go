package opencode

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// cursorData is the JSON payload stored in backend.Cursor.Data for OC.
// (time_created, message_id) is a unique key on the (ordered) message table
// and is the resume point for Phase 10's LoadIncremental. Phase 8 emits
// cursors pointing at the current leaf so Load → later LoadIncremental
// works without a placeholder migration.
type cursorData struct {
	LastTimeCreated int64  `json:"last_time_created"`
	LastMessageID   string `json:"last_message_id"`
}

// encodeCursor wraps cursorData in the source-tagged backend.Cursor envelope.
func encodeCursor(cd cursorData) backend.Cursor {
	data, _ := json.Marshal(cd)
	return backend.Cursor{Kind: session.SourceOpenCode, Data: data}
}

// decodeCursor unwraps a backend.Cursor to cursorData. Returns an error on
// kind mismatch so a cross-source cursor mix-up surfaces loudly.
func decodeCursor(c backend.Cursor) (cursorData, error) {
	if c.Kind != session.SourceOpenCode {
		return cursorData{}, fmt.Errorf("cursor kind mismatch: got %q want %q", c.Kind, session.SourceOpenCode)
	}
	if len(c.Data) == 0 {
		return cursorData{}, nil
	}
	var cd cursorData
	if err := json.Unmarshal(c.Data, &cd); err != nil {
		return cursorData{}, fmt.Errorf("decode opencode cursor: %w", err)
	}
	return cd, nil
}

// msToTime converts Unix milliseconds to time.Time. Zero in → zero out.
func msToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
