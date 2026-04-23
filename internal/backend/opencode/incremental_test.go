package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/session"
)

// mutate opens a writable connection to dbPath (the store's own connection
// is read-only) and runs the given SQL. Used by tests that simulate OC
// writing new rows between ticks.
func mutate(t *testing.T, dbPath, sqlText string) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(sqlText)
	require.NoError(t, err)
}

// copyFixture copies a testdata DB to a temp path so tests that mutate the
// DB don't pollute the shared fixture.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := testdataPath(t, name)
	dst := filepath.Join(t.TempDir(), name)

	in, err := os.Open(src)
	require.NoError(t, err)
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	require.NoError(t, err)
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	require.NoError(t, err)
	return dst
}

func TestLoadIncremental_NewMessage_ReturnsIncremental(t *testing.T) {
	// Arrange: open a fresh copy, Load to capture the cursor.
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, cur, err := store.Load(context.Background(), "ses_s1")
	require.NoError(t, err)
	require.Equal(t, session.SourceOpenCode, cur.Kind)

	// Mutate via a separate writable connection (store's handle is read-only).
	mutate(t, dbPath, `
		INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
			('msg_new_u', 'ses_s1', 1700001060000, 1700001060000,
			 '{"role":"user"}'),
			('msg_new_a', 'ses_s1', 1700001070000, 1700001070000,
			 '{"role":"assistant","parentID":"msg_new_u","tokens":{"input":5,"output":5,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}');
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
			('prt_new_u', 'msg_new_u', 'ses_s1', 1700001060000, 1700001060000,
			 '{"type":"text","text":"another prompt"}'),
			('prt_new_a', 'msg_new_a', 'ses_s1', 1700001070000, 1700001070000,
			 '{"type":"text","text":"new reply"}');
	`)

	// Act
	newTurns, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", cur)

	// Assert
	require.NoError(t, err)
	require.Len(t, newTurns, 2)
	assert.Equal(t, "another prompt", newTurns[0].Content)
	assert.Equal(t, "new reply", newTurns[1].Content)
	assert.NotEqual(t, cur.Data, newCur.Data, "cursor must advance past the new rows")
}

func TestLoadIncremental_NoNewMessages_ReturnsEmpty(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, cur, err := store.Load(context.Background(), "ses_s1")
	require.NoError(t, err)

	// No mutations.
	turns, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", cur)

	require.NoError(t, err)
	assert.Empty(t, turns)
	assert.Equal(t, cur.Data, newCur.Data, "cursor unchanged when nothing new")
}

func TestLoadIncremental_EmptyCursor_FallsBackToLoad(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Zero-value cursor.
	empty := encodeCursor(cursorData{})

	turns, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", empty)

	require.NoError(t, err)
	assert.Len(t, turns, 4, "cold cursor must return the full chain")
	assert.NotEmpty(t, newCur.Data)
}

func TestLoadIncremental_KindMismatch_Error(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	bad := encodeCursor(cursorData{LastMessageID: "x", LastTimeCreated: 1})
	bad.Kind = session.SourceClaude

	_, _, err = store.LoadIncremental(context.Background(), "ses_s1", bad)

	assert.Error(t, err)
}

func TestLoadRange_HappyPath(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	turns, err := store.LoadRange(context.Background(), "ses_s1", 1, 3)

	require.NoError(t, err)
	assert.Len(t, turns, 2, "LoadRange(1,3) returns 2 turns")
	assert.Equal(t, session.RoleAssistant, turns[0].Role)
}

func TestLoadRange_ClampsToLength(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// ses_s1 has 4 turns; ask for 2..999.
	turns, err := store.LoadRange(context.Background(), "ses_s1", 2, 999)

	require.NoError(t, err)
	assert.Len(t, turns, 2)
}

func TestLoadRange_FromPastEnd_EmptyNoError(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	turns, err := store.LoadRange(context.Background(), "ses_s1", 999, 1000)

	require.NoError(t, err)
	assert.Empty(t, turns)
}

func TestLoadRange_InvalidRanges_Error(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.LoadRange(context.Background(), "ses_s1", -1, 5)
	assert.Error(t, err)

	_, err = store.LoadRange(context.Background(), "ses_s1", 3, 1)
	assert.Error(t, err)
}
