package claude_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justincordova/seshr/internal/backend"
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

// TestStore_LoadIncremental_PartialTrailingRecordAtLoad guards against a
// data-loss race: if the live agent is mid-write when Load runs, the file ends
// with a partial (newline-less) record. Load must anchor ByteOffset to the last
// complete record boundary, not the raw file size, so that when the writer
// finishes the record the next LoadIncremental re-reads it whole rather than
// seeking into its middle.
func TestStore_LoadIncremental_PartialTrailingRecordAtLoad(t *testing.T) {
	// Arrange
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	path := filepath.Join(proj, "sess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), path))

	// Simulate the agent having written a partial trailing record (no newline).
	partial := `{"type":"user","message":{"role":"user","content":"MID_WRITE_`
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(partial)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	store := claudeBackend.NewStore(root)
	_, cur, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)

	// The writer now completes the in-flight record and adds one more.
	rest := `MARKER","content2":"x"},"uuid":"umid","parentUuid":null,"timestamp":"2026-03-20T12:00:00.000Z","sessionId":"sess-simple"}` + "\n"
	next := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]},"uuid":"anext","parentUuid":"umid","timestamp":"2026-03-20T12:00:05.000Z","sessionId":"sess-simple"}` + "\n"
	f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(rest + next)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Act
	turns, _, err := store.LoadIncremental(context.Background(), "sess", cur)

	// Assert: both the completed mid-write record and the following one are
	// read, and the mid-write record is intact (prefix preserved).
	require.NoError(t, err)
	require.Len(t, turns, 2, "completed mid-write record must not be lost")
	assert.True(t, strings.HasPrefix(turns[0].Content, "MID_WRITE_MARKER"),
		"the record being written at Load time must be re-read whole")
}

// TestStore_LoadIncremental_RotationReturnsErrCursorInvalid guards the
// incremental contract: when the file shrinks (prune replace, truncation),
// LoadIncremental must NOT return the full turn list — its callers append,
// which would duplicate every turn — but signal a required full reload.
func TestStore_LoadIncremental_RotationReturnsErrCursorInvalid(t *testing.T) {
	// Arrange
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	path := filepath.Join(proj, "sess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), path))

	store := claudeBackend.NewStore(root)
	_, cur, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)

	// Simulate a prune-style replace: rewrite the file smaller.
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"only"},"uuid":"u1","timestamp":"2026-03-20T10:00:00.000Z","sessionId":"sess-simple"}`+"\n"), 0o644))

	// Act
	turns, _, err := store.LoadIncremental(context.Background(), "sess", cur)

	// Assert
	assert.ErrorIs(t, err, backend.ErrCursorInvalid)
	assert.Empty(t, turns)
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

// TestStore_LoadRange_SameIndexSpaceAsLoad guards against index drift between
// LoadRange and Load: attached tool_result records are folded into their
// assistant turn by Parse, so a range scan that counted them as standalone
// turns would return earlier turns than asked for.
func TestStore_LoadRange_SameIndexSpaceAsLoad(t *testing.T) {
	// Arrange: fixture with tool_use records whose results attach to turns.
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "multi_topic.jsonl"), filepath.Join(proj, "sess.jsonl")))

	store := claudeBackend.NewStore(root)
	sess, _, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(sess.Turns), 4)

	// Act
	turns, err := store.LoadRange(context.Background(), "sess", 2, 4)

	// Assert: identical turns, not just identical count.
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Equal(t, sess.Turns[2].Content, turns[0].Content)
	assert.Equal(t, sess.Turns[2].RawIndex, turns[0].RawIndex)
	assert.Equal(t, sess.Turns[3].Content, turns[1].Content)
	assert.Equal(t, sess.Turns[3].RawIndex, turns[1].RawIndex)
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
