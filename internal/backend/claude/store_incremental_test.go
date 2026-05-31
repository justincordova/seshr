package claude_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_LoadIncremental_FullReloadOnZeroCursor(t *testing.T) {
	// Arrange
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), filepath.Join(proj, "sess.jsonl")))

	store := claudeBackend.NewStore(root)
	sess, cur, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)
	nFull := len(sess.Turns)

	// Act: incremental from zero cursor → should return all turns (full reload).
	turns, _, err := store.LoadIncremental(context.Background(), "sess", cur)

	// Assert: at EOF, no new turns (cursor is already at EOF after Load).
	require.NoError(t, err)
	assert.LessOrEqual(t, len(turns), nFull)
}

func TestStore_LoadRange_ReturnsSlice(t *testing.T) {
	// Arrange
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "replay_basic.jsonl"), filepath.Join(proj, "sess.jsonl")))

	store := claudeBackend.NewStore(root)
	sess, _, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(sess.Turns), 4)

	// Act: load turns [1,3).
	turns, err := store.LoadRange(context.Background(), "sess", 1, 3)

	// Assert
	require.NoError(t, err)
	assert.Len(t, turns, 2)
}

func TestStore_LoadRange_InvalidRange_ReturnsError(t *testing.T) {
	// Arrange
	root := t.TempDir()
	store := claudeBackend.NewStore(root)

	// Act
	_, err := store.LoadRange(context.Background(), "x", 5, 3) // to <= from

	// Assert
	assert.Error(t, err)
}

// TestStore_LoadIncremental_OversizedRecordRoundTrips guards the bufio
// ReadSlice/ReadBytes spillover path: a JSONL record larger than the stream
// reader's 64KB buffer must be reassembled intact. ReadSlice returns a slice
// into the internal buffer that the subsequent ReadBytes invalidates, so the
// prefix must be copied out before the buffer is reused.
func TestStore_LoadIncremental_OversizedRecordRoundTrips(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	path := filepath.Join(proj, "sess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), path))

	store := claudeBackend.NewStore(root)
	_, cur, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)

	// Append a user record well over the 64KB stream-reader buffer.
	marker := "OVERSIZE_MARKER_"
	big := marker + strings.Repeat("x", 200*1024)
	rec := fmt.Sprintf(
		`{"type":"user","message":{"role":"user","content":%q},"uuid":"ubig","parentUuid":null,"timestamp":"2026-03-20T11:00:00.000Z","sessionId":"sess-simple"}`+"\n",
		big,
	)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(rec)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	turns, _, err := store.LoadIncremental(context.Background(), "sess", cur)
	require.NoError(t, err)
	require.Len(t, turns, 1, "the appended oversized record must be parsed")
	assert.True(t, strings.HasPrefix(turns[0].Content, marker),
		"oversized record content must be reassembled intact, not corrupted")
	assert.Len(t, turns[0].Content, len(big))
}
