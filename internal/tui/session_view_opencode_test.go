package tui_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/backend"
	ocBackend "github.com/justincordova/seshr/internal/backend/opencode"
	"github.com/justincordova/seshr/internal/tui"
)

// copyOCFixture places a writable copy of testdata/<name> in a new temp dir
// so mutation tests can INSERT without perturbing the shared fixture.
func copyOCFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("../../testdata", name)
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

// insertOCRows opens a writable handle to dbPath (SessionView's store holds a
// read-only one) and runs sqlText.
func insertOCRows(t *testing.T, dbPath, sqlText string) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(sqlText)
	require.NoError(t, err)
}

// TestSessionView_LiveTail_OpenCode exercises the full Phase 10 flow at the
// tui boundary: open a SessionView against an OC store, simulate the store
// receiving new messages, then call LoadIncremental + Append as the
// fast-tick handler would.
func TestSessionView_LiveTail_OpenCode(t *testing.T) {
	dbPath := copyOCFixture(t, "opencode_simple.db")
	store, err := ocBackend.NewStore(dbPath, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	meta := backend.SessionMeta{ID: "ses_s1", Kind: store.Kind()}
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)

	initial := len(view.Session.Turns)
	require.Equal(t, 4, initial, "simple fixture has 4 turns in ses_s1")

	// Simulate OpenCode writing a new user/assistant pair to the DB.
	insertOCRows(t, dbPath, `
		INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
			('msg_live_u', 'ses_s1', 1700001061000, 1700001061000, '{"role":"user"}'),
			('msg_live_a', 'ses_s1', 1700001062000, 1700001062000,
			 '{"role":"assistant","parentID":"msg_live_u","tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}');
		INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
			('prt_live_u', 'msg_live_u', 'ses_s1', 1700001061000, 1700001061000,
			 '{"type":"text","text":"live prompt"}'),
			('prt_live_a', 'msg_live_a', 'ses_s1', 1700001062000, 1700001062000,
			 '{"type":"text","text":"live reply"}');
	`)

	// Snapshot the initial cursor; Append mutates view.Cursor.
	prevCursor := view.Cursor

	// This is the call the fast-tick handler makes.
	turns, newCur, err := store.LoadIncremental(context.Background(), meta.ID, view.Cursor)
	require.NoError(t, err)
	require.Len(t, turns, 2)

	view.Append(turns, newCur)

	assert.Equal(t, initial+2, len(view.Session.Turns))
	assert.Equal(t, "live reply", view.Session.Turns[len(view.Session.Turns)-1].Content)
	assert.NotEqual(t, prevCursor.Data, view.Cursor.Data, "view cursor must advance past the new rows")
}
