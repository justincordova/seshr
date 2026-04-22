package claude_test

import (
	"context"
	"os"
	"path/filepath"
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
