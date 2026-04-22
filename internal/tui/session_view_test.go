package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/seshr/internal/backend"
	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestStore(t *testing.T, fixture string) (backend.SessionStore, backend.SessionMeta, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	dst := filepath.Join(proj, "sess.jsonl")
	src := filepath.Join("../../testdata", fixture)
	in, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, in, 0o644))
	store := claudeBackend.NewStore(root)
	meta := backend.SessionMeta{ID: "sess", Kind: session.SourceClaude}
	return store, meta, root
}

func TestSessionView_NewSessionView_LoadsAndClusters(t *testing.T) {
	// Arrange
	store, meta, _ := makeTestStore(t, "simple.jsonl")

	// Act
	view, err := tui.NewSessionView(context.Background(), store, meta)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.NotNil(t, view.Session)
	assert.NotEmpty(t, view.Session.Turns)
	assert.NotEmpty(t, view.Topics)
	assert.Equal(t, len(view.Session.Turns), view.TurnsLoadedTo)
	assert.Equal(t, 0, view.TurnsLoadedFrom)
	assert.Equal(t, len(view.Session.Turns), view.TotalTurns)
}

func TestSessionView_Append_EvictsWhenOverMax(t *testing.T) {
	// Arrange — synthesize a view with many turns.
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)

	// Fill the window to 600 turns.
	bigTurns := make([]session.Turn, 600)
	for i := range bigTurns {
		bigTurns[i] = session.Turn{Role: session.RoleUser, Content: "t"}
	}
	view.Session.Turns = bigTurns
	view.TurnsLoadedFrom = 0
	view.TurnsLoadedTo = 600
	view.TotalTurns = 600

	// Act: append 1 more turn.
	extra := []session.Turn{{Role: session.RoleAssistant, Content: "x"}}
	view.Append(extra, view.Cursor)

	// Assert: window holds exactly 500 turns.
	assert.Equal(t, 500, view.TurnsLoadedTo-view.TurnsLoadedFrom)
	assert.Equal(t, 601, view.TotalTurns)
}

func TestSessionView_HasTurn_RespectsWindow(t *testing.T) {
	// Arrange
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)

	// Act / Assert
	assert.True(t, view.HasTurn(0))
	assert.False(t, view.HasTurn(view.TurnsLoadedTo))
}
