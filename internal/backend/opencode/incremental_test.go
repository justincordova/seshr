package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	res, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", cur)

	// Assert
	require.NoError(t, err)
	require.Len(t, res.Turns, 2)
	assert.Equal(t, "another prompt", res.Turns[0].Content)
	assert.Equal(t, "new reply", res.Turns[1].Content)
	assert.NotEqual(t, cur.Data, newCur.Data, "cursor must advance past the new rows")
}

func TestLoadIncremental_InFlightAssistant_HeldBackUntilCompleted(t *testing.T) {
	// Arrange
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, cur, err := store.Load(context.Background(), "ses_s1")
	require.NoError(t, err)

	// OC starts a step: assistant message row exists (time.created set, no
	// time.completed) with only an early text part — output still streaming.
	// The timestamp must be recent: in-flight status expires after
	// inFlightHoldback so crashed sessions don't hide their last turn.
	nowMs := time.Now().UnixMilli()
	mutate(t, dbPath, fmt.Sprintf(`
		INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
			('msg_live_a', 'ses_s1', %[1]d, %[1]d,
			 '{"role":"assistant","parentID":"msg_a2","time":{"created":%[1]d},"tokens":{"input":5,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}');
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
			('prt_live_1', 'msg_live_a', 'ses_s1', %[1]d, %[1]d,
			 '{"type":"text","text":"first chunk"}');
	`, nowMs))

	// Act 1: the in-flight message must be held back, cursor unmoved.
	res, midCur, err := store.LoadIncremental(context.Background(), "ses_s1", cur)
	require.NoError(t, err)
	assert.Empty(t, res.Turns, "streaming assistant message must not be emitted yet")
	assert.Equal(t, cur.Data, midCur.Data, "cursor must hold before the in-flight message")

	// OC finishes the step: more parts land and time.completed is written.
	mutate(t, dbPath, fmt.Sprintf(`
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
			('prt_live_2', 'msg_live_a', 'ses_s1', %[1]d, %[1]d,
			 '{"type":"text","text":"second chunk"}');
		UPDATE message SET data = '{"role":"assistant","parentID":"msg_a2","time":{"created":%[2]d,"completed":%[1]d},"tokens":{"input":5,"output":9,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}'
		WHERE id = 'msg_live_a';
	`, nowMs+1000, nowMs))

	// Act 2: now it must be emitted whole — including the late part.
	res, _, err = store.LoadIncremental(context.Background(), "ses_s1", midCur)
	require.NoError(t, err)
	require.Len(t, res.Turns, 1)
	assert.Contains(t, res.Turns[0].Content, "first chunk")
	assert.Contains(t, res.Turns[0].Content, "second chunk",
		"parts written after the message row was first observed must not be lost")
}

func TestLoad_TrailingInFlightAssistant_ExcludedAndCursorBeforeIt(t *testing.T) {
	// Arrange: the session ends with a recently started streaming assistant.
	dbPath := copyFixture(t, "opencode_simple.db")
	nowMs := time.Now().UnixMilli()
	mutate(t, dbPath, fmt.Sprintf(`
		INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
			('msg_live_a', 'ses_s1', %[1]d, %[1]d,
			 '{"role":"assistant","parentID":"msg_a2","time":{"created":%[1]d},"tokens":{"input":5,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}');
	`, nowMs))
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Act
	sess, cur, err := store.Load(context.Background(), "ses_s1")

	// Assert: the in-flight turn is held back and the cursor sits before it,
	// so the incremental path can emit it whole once it completes.
	require.NoError(t, err)
	assert.Len(t, sess.Turns, 4, "in-flight assistant must not appear as a frozen turn")
	cd, err := decodeCursor(cur)
	require.NoError(t, err)
	assert.NotEqual(t, "msg_live_a", cd.LastMessageID)
}

func TestLoad_AbandonedInFlightAssistant_StillEmitted(t *testing.T) {
	// Arrange: the agent died mid-step long ago — time.completed was never
	// written. The hold-back must expire or this turn is invisible forever.
	dbPath := copyFixture(t, "opencode_simple.db")
	mutate(t, dbPath, `
		INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
			('msg_dead_a', 'ses_s1', 1700001080000, 1700001080000,
			 '{"role":"assistant","parentID":"msg_a2","time":{"created":1700001080000},"tokens":{"input":5,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}');
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
			('prt_dead_1', 'msg_dead_a', 'ses_s1', 1700001080001, 1700001080001,
			 '{"type":"text","text":"last words"}');
	`)
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Act
	sess, _, err := store.Load(context.Background(), "ses_s1")

	// Assert
	require.NoError(t, err)
	require.Len(t, sess.Turns, 5, "abandoned partial turn must still be emitted")
	assert.Contains(t, sess.Turns[4].Content, "last words")
}

func TestLoadIncremental_NoNewMessages_ReturnsEmpty(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, cur, err := store.Load(context.Background(), "ses_s1")
	require.NoError(t, err)

	// No mutations.
	res, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", cur)

	require.NoError(t, err)
	assert.Empty(t, res.Turns)
	assert.Equal(t, cur.Data, newCur.Data, "cursor unchanged when nothing new")
}

func TestLoadIncremental_EmptyCursor_FallsBackToLoad(t *testing.T) {
	dbPath := copyFixture(t, "opencode_simple.db")
	store, err := NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Zero-value cursor.
	empty := encodeCursor(cursorData{})

	res, newCur, err := store.LoadIncremental(context.Background(), "ses_s1", empty)

	require.NoError(t, err)
	assert.Len(t, res.Turns, 4, "cold cursor must return the full chain")
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
