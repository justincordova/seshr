package opencode

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/backend"
)

// newEditorTest returns an Editor with its own writable DB copy and a
// backup directory. The store+editor point at the SAME file, so changes
// made through the editor are visible through store.Load.
func newEditorTest(t *testing.T, fixture string) (*Editor, *Store, string) {
	t.Helper()
	src := testdataPath(t, fixture)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, fixture)

	in, err := os.Open(src)
	require.NoError(t, err)
	defer func() { _ = in.Close() }()
	out, err := os.Create(dbPath)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())

	backups := filepath.Join(dir, "backups")
	store, err := NewStore(dbPath, backups)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ed := NewEditor(store, backups)
	ed.now = func() time.Time { return time.UnixMilli(1_700_000_000_000) }
	return ed, store, dbPath
}

func TestEditor_Prune_HappyPath_RemovesTurnsAndWritesBackup(t *testing.T) {
	ed, store, _ := newEditorTest(t, "opencode_simple.db")

	// Session ses_s1 has 4 turns; prune index 0 (the first user msg).
	result, err := ed.Prune(context.Background(), "ses_s1", backend.Selection{TurnIndices: []int{0}})
	require.NoError(t, err)
	assert.Equal(t, 0, result.SkippedRunningTools)

	// Re-Load the session; first turn must be gone.
	sess, _, err := store.Load(context.Background(), "ses_s1")
	require.NoError(t, err)
	assert.Len(t, sess.Turns, 3, "expected 3 turns after pruning 1 of 4")

	// Backup file must exist.
	entries, err := os.ReadDir(ed.sessionBackupDir("ses_s1"))
	require.NoError(t, err)
	jsonFiles := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonFiles++
		}
	}
	assert.GreaterOrEqual(t, jsonFiles, 1)
}

func TestEditor_Prune_LiveToolPart_SkippedAndCounted(t *testing.T) {
	ed, store, _ := newEditorTest(t, "opencode_with_tools.db")

	// Session has 4 turns; turn 3 (index 3) is the assistant with
	// running+pending tools. Pruning that turn should preserve the live
	// parts and keep the message (since all its parts are live).
	result, err := ed.Prune(context.Background(), "ses_tl", backend.Selection{TurnIndices: []int{3}})
	require.NoError(t, err)
	assert.Equal(t, 2, result.SkippedRunningTools, "expected running + pending skipped")

	// The target turn is preserved because every one of its parts is live.
	sess, _, err := store.Load(context.Background(), "ses_tl")
	require.NoError(t, err)
	require.Len(t, sess.Turns, 4)
	assert.Len(t, sess.Turns[3].ToolCalls, 2)
}

func TestEditor_Prune_EmptySelection_NoOp(t *testing.T) {
	ed, _, _ := newEditorTest(t, "opencode_simple.db")

	result, err := ed.Prune(context.Background(), "ses_s1", backend.Selection{})

	require.NoError(t, err)
	assert.Equal(t, 0, result.SkippedRunningTools)
}

func TestEditor_Prune_InvalidIndex_Error(t *testing.T) {
	ed, _, _ := newEditorTest(t, "opencode_simple.db")

	_, err := ed.Prune(context.Background(), "ses_s1", backend.Selection{TurnIndices: []int{99}})

	assert.Error(t, err)
}

func TestEditor_Delete_RemovesSessionAndCascades(t *testing.T) {
	ed, store, _ := newEditorTest(t, "opencode_simple.db")

	// Act: delete ses_s2.
	require.NoError(t, ed.Delete(context.Background(), "ses_s2"))

	// Session gone from Scan.
	metas, err := store.Scan(context.Background())
	require.NoError(t, err)
	for _, m := range metas {
		assert.NotEqual(t, "ses_s2", m.ID, "deleted session must not appear in Scan")
	}

	// Load returns an empty shell.
	sess, _, err := store.Load(context.Background(), "ses_s2")
	require.NoError(t, err)
	assert.Empty(t, sess.Turns)

	// Delete-mode backup exists.
	entries, err := os.ReadDir(ed.sessionBackupDir("ses_s2"))
	require.NoError(t, err)
	var deleteBackups []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" && contains(e.Name(), "-delete") {
			deleteBackups = append(deleteBackups, e.Name())
		}
	}
	assert.NotEmpty(t, deleteBackups)
}

func TestEditor_RestoreBackup_AfterDelete_ReinsertsRows(t *testing.T) {
	ed, store, _ := newEditorTest(t, "opencode_simple.db")

	// Delete ses_s2, then restore.
	require.NoError(t, ed.Delete(context.Background(), "ses_s2"))
	require.NoError(t, ed.RestoreBackup(context.Background(), "ses_s2"))

	// Session metadata should reappear.
	metas, err := store.Scan(context.Background())
	require.NoError(t, err)
	var found bool
	for _, m := range metas {
		if m.ID == "ses_s2" {
			found = true
		}
	}
	assert.True(t, found, "restored session must appear in Scan")

	// Turn content roundtrips.
	sess, _, err := store.Load(context.Background(), "ses_s2")
	require.NoError(t, err)
	assert.Len(t, sess.Turns, 2)
	assert.Equal(t, "what time is it", sess.Turns[0].Content)
}

func TestEditor_RestoreBackup_Twice_IsNoOp(t *testing.T) {
	ed, _, _ := newEditorTest(t, "opencode_simple.db")

	require.NoError(t, ed.Delete(context.Background(), "ses_s2"))
	require.NoError(t, ed.RestoreBackup(context.Background(), "ses_s2"))
	require.NoError(t, ed.RestoreBackup(context.Background(), "ses_s2"),
		"restore on already-restored session must be idempotent")
}

func TestEditor_RestoreBackup_NoBackup_Error(t *testing.T) {
	ed, _, _ := newEditorTest(t, "opencode_simple.db")

	err := ed.RestoreBackup(context.Background(), "ses_nonexistent")

	assert.Error(t, err)
}

func TestEditor_HasBackup(t *testing.T) {
	ed, _, _ := newEditorTest(t, "opencode_simple.db")

	assert.False(t, ed.HasBackup("ses_s1"))

	// Prune creates a backup.
	_, err := ed.Prune(context.Background(), "ses_s1", backend.Selection{TurnIndices: []int{0}})
	require.NoError(t, err)

	assert.True(t, ed.HasBackup("ses_s1"))
}

func TestApplyRetention_KeepsLastN(t *testing.T) {
	dir := t.TempDir()
	// Create 7 timestamped backups.
	for i := 0; i < 7; i++ {
		name := filepath.Join(dir, formatTimestampFor(i)+".json")
		require.NoError(t, os.WriteFile(name, []byte("{}"), 0o600))
	}

	require.NoError(t, applyRetention(dir, 5))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var jsons []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsons = append(jsons, e.Name())
		}
	}
	assert.Len(t, jsons, 5, "retention must cap at 5")
}

func TestWriteAndRetainBackup_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	ed := &Editor{backupDir: dir, now: func() time.Time { return time.UnixMilli(1_700_000_000_000) }}

	err := ed.writeAndRetainBackup("ses_x", "prune",
		[]messageRowBak{{ID: "m1", SessionID: "ses_x", TimeCreated: 1, Data: json.RawMessage(`{}`)}},
		nil,
	)

	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Join(dir, "ses_x"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestWithSessionLock_SecondAcquireFails(t *testing.T) {
	dir := t.TempDir()
	ed := &Editor{backupDir: dir, now: time.Now}

	acquired := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- ed.withSessionLock("ses_x", func() error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired

	// Second caller must fail fast with ErrConcurrentPrune.
	err := ed.withSessionLock("ses_x", func() error { return nil })
	assert.ErrorIs(t, err, ErrConcurrentPrune)

	close(release)
	assert.NoError(t, <-done)
}

func TestPartIsLiveTool_AllFourStatuses(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"running", true},
		{"pending", true},
		{"completed", false},
		{"error", false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			raw := []byte(`{"type":"tool","state":{"status":"` + tc.status + `"}}`)
			assert.Equal(t, tc.want, partIsLiveTool(raw))
		})
	}
}

func TestPartIsLiveTool_NonToolPart_False(t *testing.T) {
	assert.False(t, partIsLiveTool([]byte(`{"type":"text"}`)))
	assert.False(t, partIsLiveTool([]byte(`not json`)))
}

func TestLatestBackupPath_PicksLexicallyLargest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"20200101-000000.json", "20230101-000000.json", "20210101-000000.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600))
	}

	got, err := latestBackupPath(dir)

	require.NoError(t, err)
	assert.Contains(t, got, "20230101")
}

// ── test helpers ─────────────────────────────────────────────────────────

func formatTimestampFor(i int) string {
	// Produce a unique, sortable timestamp per call.
	return time.Date(2026, time.April, 23, 0, 0, i, 0, time.UTC).Format("20060102-150405")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
