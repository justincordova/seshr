package claude

import (
	"encoding/json"
	"fmt"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// cursorData holds file-identity state for incremental JSONL loading.
type cursorData struct {
	ByteOffset int64  `json:"offset"`
	MtimeNs    int64  `json:"mtime_ns"`
	SizeBytes  int64  `json:"size"`            // Darwin: file size as identity proxy
	Inode      uint64 `json:"inode,omitempty"` // Linux: inode
}

// encodeCursor marshals a cursorData into a backend.Cursor.
func encodeCursor(cd cursorData) backend.Cursor {
	data, _ := json.Marshal(cd)
	return backend.Cursor{Kind: session.SourceClaude, Data: data}
}

// decodeCursor unmarshals a backend.Cursor into cursorData.
func decodeCursor(c backend.Cursor) (cursorData, error) {
	if len(c.Data) == 0 || string(c.Data) == "{}" {
		return cursorData{}, nil
	}
	var cd cursorData
	if err := json.Unmarshal(c.Data, &cd); err != nil {
		return cursorData{}, fmt.Errorf("decode cursor: %w", err)
	}
	return cd, nil
}

// identitiesMatch returns true when the prev cursor identifies the same file
// as current (no rotation has occurred).
func identitiesMatch(prev, current cursorData) bool {
	if prev.MtimeNs == 0 {
		return false // zero cursor → always reload
	}
	// If inode is set on both, use it.
	if prev.Inode != 0 && current.Inode != 0 {
		return prev.Inode == current.Inode && prev.MtimeNs <= current.MtimeNs
	}
	// Darwin fallback: size identity (file rotation typically produces a
	// different size; mtime alone can revert on rename).
	return prev.SizeBytes == current.SizeBytes && prev.MtimeNs <= current.MtimeNs
}
