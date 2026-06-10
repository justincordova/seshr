package claude_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditor_HasBackup_False_WhenNoBackupExists(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), filepath.Join(projDir, "mysession.jsonl")))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)

	// Act / Assert
	assert.False(t, ed.HasBackup("mysession"))
}

func TestEditor_HasBackup_True_WhenBackupExists(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	src := filepath.Join(testdataDir, "simple.jsonl")
	dst := filepath.Join(projDir, "mysession.jsonl")
	require.NoError(t, copyFile(src, dst))
	require.NoError(t, copyFile(src, dst+".bak"))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)

	// Act / Assert
	assert.True(t, ed.HasBackup("mysession"))
}

func TestEditor_PruneAndRestore_RoundTrip(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	dst := filepath.Join(projDir, "prunesess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "prune_basic.jsonl"), dst))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)

	// Act: prune turn 0
	_, err := ed.Prune(context.Background(), "prunesess", backend.Selection{TurnIndices: []int{0}})
	require.NoError(t, err)

	// Assert: backup created
	assert.True(t, ed.HasBackup("prunesess"))

	// Restore
	require.NoError(t, ed.RestoreBackup(context.Background(), "prunesess"))

	// Load restored session — should parse cleanly
	sess, _, err := store.Load(context.Background(), "prunesess")
	require.NoError(t, err)
	assert.NotEmpty(t, sess.Turns)
}

func TestEditor_Prune_StaleTimestamps_Aborts(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	dst := filepath.Join(projDir, "prunesess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "prune_basic.jsonl"), dst))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)

	// Act: a selection whose timestamp doesn't match turn 0 — as if the
	// transcript was rewritten after the UI computed the selection.
	_, err := ed.Prune(context.Background(), "prunesess", backend.Selection{
		TurnIndices:    []int{0},
		TurnTimestamps: []time.Time{time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)},
	})

	// Assert: refused, file untouched, no backup taken.
	assert.ErrorIs(t, err, backend.ErrSelectionStale)
	assert.False(t, ed.HasBackup("prunesess"))
}

func TestEditor_Prune_MatchingTimestamps_Succeeds(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	dst := filepath.Join(projDir, "prunesess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "prune_basic.jsonl"), dst))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)
	sess, _, err := store.Load(context.Background(), "prunesess")
	require.NoError(t, err)

	// Act: timestamps taken from the loaded session, like the TUI does.
	_, err = ed.Prune(context.Background(), "prunesess", backend.Selection{
		TurnIndices:    []int{0},
		TurnTimestamps: []time.Time{sess.Turns[0].Timestamp},
	})

	// Assert
	require.NoError(t, err)
	after, _, err := store.Load(context.Background(), "prunesess")
	require.NoError(t, err)
	assert.Less(t, len(after.Turns), len(sess.Turns))
}

func TestEditor_Delete_RemovesFile(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	dst := filepath.Join(projDir, "sess.jsonl")
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), dst))

	store := claudeBackend.NewStore(root)
	ed := claudeBackend.NewEditor(store)

	// Act
	require.NoError(t, ed.Delete(context.Background(), "sess"))

	// Assert
	_, err := os.Stat(dst)
	assert.True(t, os.IsNotExist(err))
}
