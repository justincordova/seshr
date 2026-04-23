package opencode

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/session"
)

// testdataPath resolves testdata/<file> relative to the repo root regardless
// of the test's working directory.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// thisFile is .../internal/backend/opencode/store_test.go; repo root is
	// three directories up from there.
	repo := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	return filepath.Join(repo, "testdata", name)
}

func TestNewStore_MissingDB_ReturnsErrNoDatabase(t *testing.T) {
	_, err := NewStore("/nonexistent/path/opencode.db", t.TempDir())

	assert.ErrorIs(t, err, ErrNoDatabase)
}

func TestNewStore_ValidDB_ReturnsStore(t *testing.T) {
	// Arrange
	path := testdataPath(t, "opencode_simple.db")

	// Act
	store, err := NewStore(path, t.TempDir())
	t.Cleanup(func() { _ = store.Close() })

	// Assert
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Equal(t, session.SourceOpenCode, store.Kind())
}

func TestStore_Close_CanBeCalledTwice(t *testing.T) {
	path := testdataPath(t, "opencode_simple.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)

	assert.NoError(t, store.Close())
	assert.NoError(t, store.Close())
}

func TestScan_SimpleDB_ReturnsAllSessions(t *testing.T) {
	// Arrange
	path := testdataPath(t, "opencode_simple.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Act
	metas, err := store.Scan(context.Background())

	// Assert
	require.NoError(t, err)
	require.Len(t, metas, 2)

	// Most recent first (sort by time_updated desc). ses_s2 has
	// time_updated=1700002050000 vs ses_s1=1700001050000.
	assert.Equal(t, "ses_s2", metas[0].ID)
	assert.Equal(t, session.SourceOpenCode, metas[0].Kind)
	assert.Equal(t, "code", metas[0].Project) // project.name
	assert.Equal(t, "/home/user/code", metas[0].Directory)

	// ses_s1 token aggregate: 10+20 + 15+30+5+100 = 180.
	var s1 *struct {
		tokens int
		cost   float64
	}
	for _, m := range metas {
		if m.ID == "ses_s1" {
			s1 = &struct {
				tokens int
				cost   float64
			}{m.TokenCount, m.CostUSD}
		}
	}
	require.NotNil(t, s1)
	assert.Equal(t, 180, s1.tokens)
	assert.InDelta(t, 0.003, s1.cost, 1e-9)
}

func TestScan_NoArchiveIncluded(t *testing.T) {
	// Every fixture session has NULL time_archived, so this is a smoke check
	// that the WHERE clause survives future edits.
	path := testdataPath(t, "opencode_simple.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	metas, err := store.Scan(context.Background())
	require.NoError(t, err)
	for _, m := range metas {
		assert.NotZero(t, m.UpdatedAt, "session %s has zero UpdatedAt", m.ID)
	}
}

func TestLoad_SimpleDB_ReturnsTurns(t *testing.T) {
	path := testdataPath(t, "opencode_simple.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sess, cur, err := store.Load(context.Background(), "ses_s1")

	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, session.SourceOpenCode, sess.Source)
	assert.Len(t, sess.Turns, 4) // user, assistant, user, assistant
	assert.Equal(t, session.RoleUser, sess.Turns[0].Role)
	assert.Equal(t, session.RoleAssistant, sess.Turns[1].Role)
	assert.Equal(t, "hello", sess.Turns[0].Content)
	assert.Equal(t, "hi there", sess.Turns[1].Content)
	// Last assistant turn has a tool call (not a text part).
	require.Len(t, sess.Turns[3].ToolCalls, 1)
	assert.Equal(t, "bash", sess.Turns[3].ToolCalls[0].Name)
	require.Len(t, sess.Turns[3].ToolResults, 1)
	assert.Contains(t, sess.Turns[3].ToolResults[0].Content, "file1")

	// Cursor must be Kind-tagged and non-empty.
	assert.Equal(t, session.SourceOpenCode, cur.Kind)
	assert.NotEmpty(t, cur.Data)
}

func TestLoad_BranchingDB_PicksMostRecentLeaf(t *testing.T) {
	path := testdataPath(t, "opencode_branching.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sess, _, err := store.Load(context.Background(), "ses_br")

	require.NoError(t, err)
	// Expected chain: user → assistant_new (old branch dropped).
	require.Len(t, sess.Turns, 2)
	assert.Equal(t, session.RoleUser, sess.Turns[0].Role)
	assert.Equal(t, session.RoleAssistant, sess.Turns[1].Role)
	assert.Equal(t, "NEW: 42", sess.Turns[1].Content,
		"expected newest branch, got stale")
}

func TestLoad_WithTools_AllFourStatuses(t *testing.T) {
	path := testdataPath(t, "opencode_with_tools.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sess, _, err := store.Load(context.Background(), "ses_tl")
	require.NoError(t, err)
	require.Len(t, sess.Turns, 4)

	// First assistant: 2 tool calls, both paired (completed + error).
	a1 := sess.Turns[1]
	require.Equal(t, session.RoleAssistant, a1.Role)
	require.Len(t, a1.ToolCalls, 2)
	require.Len(t, a1.ToolResults, 2)
	var sawError bool
	for _, r := range a1.ToolResults {
		if r.IsError {
			sawError = true
		}
	}
	assert.True(t, sawError, "expected at least one tool_result with IsError")

	// Second assistant: 2 tool calls (running + pending), zero results.
	a2 := sess.Turns[3]
	require.Equal(t, session.RoleAssistant, a2.Role)
	require.Len(t, a2.ToolCalls, 2)
	assert.Empty(t, a2.ToolResults, "running/pending tools must NOT emit a result")
}

func TestLoad_CompactionDB_EmitsBoundary(t *testing.T) {
	path := testdataPath(t, "opencode_compaction.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sess, _, err := store.Load(context.Background(), "ses_cp")
	require.NoError(t, err)

	require.Len(t, sess.CompactBoundaries, 1)
	// Compaction part lives on the last (4th) message; turns append one per
	// message, so boundary sits at TurnIndex = 3 (the 4th turn, index 3).
	assert.Equal(t, 3, sess.CompactBoundaries[0].TurnIndex)
	assert.Equal(t, "manual", sess.CompactBoundaries[0].Trigger)
}

func TestLoad_MissingSession_ReturnsEmpty(t *testing.T) {
	path := testdataPath(t, "opencode_simple.db")
	store, err := NewStore(path, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	sess, _, err := store.Load(context.Background(), "ses_nonexistent")

	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Empty(t, sess.Turns)
}

func TestHasBackup_WithFiles(t *testing.T) {
	tmp := t.TempDir()
	sessDir := filepath.Join(tmp, "ses_x")
	require.NoError(t, runtimeMkDirAll(sessDir))
	require.NoError(t, writeFile(filepath.Join(sessDir, "20260101-000000.json"), "{}"))

	store := &Store{backupDir: tmp}
	assert.True(t, store.hasBackup("ses_x"))
	assert.False(t, store.hasBackup("ses_nope"))
}
