package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestSessionView_Append_TopicIndicesValidAfterEviction(t *testing.T) {
	// Arrange — a full window about to evict.
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)

	base := time.Unix(1_700_000_000, 0)
	bigTurns := make([]session.Turn, 600)
	for i := range bigTurns {
		bigTurns[i] = session.Turn{
			Role:      session.RoleUser,
			Content:   "t",
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
	}
	view.Session.Turns = bigTurns
	view.TurnsLoadedFrom = 0
	view.TurnsLoadedTo = 600
	view.TotalTurns = 600

	// Act: this append triggers eviction.
	view.Append([]session.Turn{{
		Role:      session.RoleAssistant,
		Content:   "x",
		Timestamp: base.Add(601 * time.Second),
	}}, view.Cursor)

	// Assert: every topic index must be a valid window-relative index whose
	// turn actually exists. Clustering before evicting would leave indices
	// shifted by the eviction excess — pruning would target the wrong turns.
	n := len(view.Session.Turns)
	require.Equal(t, 500, n)
	for ti, top := range view.Topics {
		for _, idx := range top.TurnIndices {
			assert.GreaterOrEqual(t, idx, 0, "topic %d", ti)
			assert.Less(t, idx, n, "topic %d index must be window-relative", ti)
		}
	}
	// The newest turn must be reachable through the last topic.
	last := view.Topics[len(view.Topics)-1]
	lastIdx := last.TurnIndices[len(last.TurnIndices)-1]
	assert.Equal(t, "x", view.Session.Turns[lastIdx].Content)
}

func TestSessionView_Append_AttachesToolResultToPriorTurn(t *testing.T) {
	// Arrange: the view holds an assistant turn that issued tool call t9.
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)
	view.Session.Turns = append(view.Session.Turns, session.Turn{
		Role:      session.RoleAssistant,
		Content:   "running tool",
		ToolCalls: []session.ToolCall{{ID: "t9", Name: "Bash"}},
	})
	view.TotalTurns++
	view.TurnsLoadedTo = view.TotalTurns
	before := view.TotalTurns

	// Act: the next tick streams the tool result as a standalone turn —
	// exactly what the incremental parser produces.
	view.Append([]session.Turn{{
		Role:        session.RoleToolResult,
		Content:     "tool output",
		ToolResults: []session.ToolResult{{ID: "t9", Content: "tool output"}},
	}}, view.Cursor)

	// Assert: folded into the issuing turn, not appended — matching the turn
	// index space of a fresh full parse.
	assert.Equal(t, before, view.TotalTurns, "attached result must not become a standalone turn")
	last := view.Session.Turns[len(view.Session.Turns)-1]
	require.Len(t, last.ToolResults, 1)
	assert.Equal(t, "tool output", last.ToolResults[0].Content)
}

func TestSessionView_Append_PartialMatchDoesNotDuplicate(t *testing.T) {
	// Arrange: the view holds an assistant turn that issued ONLY t1; the
	// streamed result turn answers t1 AND t2 (t2's issuer was evicted).
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)
	view.Session.Turns = append(view.Session.Turns, session.Turn{
		Role:      session.RoleAssistant,
		Content:   "running tool",
		ToolCalls: []session.ToolCall{{ID: "t1", Name: "Bash"}},
	})
	view.TotalTurns++
	view.TurnsLoadedTo = view.TotalTurns
	asstIdx := len(view.Session.Turns) - 1
	before := view.TotalTurns

	// Act
	view.Append([]session.Turn{{
		Role:    session.RoleToolResult,
		Content: "out1",
		ToolResults: []session.ToolResult{
			{ID: "t1", Content: "out1"},
			{ID: "t2", Content: "out2"},
		},
	}}, view.Cursor)

	// Assert: kept standalone (not every result found a home), and the
	// issuing turn must NOT have absorbed a duplicate copy of t1's result.
	assert.Equal(t, before+1, view.TotalTurns)
	assert.Empty(t, view.Session.Turns[asstIdx].ToolResults,
		"partial match must not eagerly fold results that are also kept standalone")
}

func TestSessionView_Append_KeepsUnmatchedToolResult(t *testing.T) {
	// Arrange
	store, meta, _ := makeTestStore(t, "simple.jsonl")
	view, err := tui.NewSessionView(context.Background(), store, meta)
	require.NoError(t, err)
	before := view.TotalTurns

	// Act: a result whose tool call doesn't exist anywhere in the view
	// (e.g. the issuing turn was evicted) stays standalone.
	view.Append([]session.Turn{{
		Role:        session.RoleToolResult,
		Content:     "orphan",
		ToolResults: []session.ToolResult{{ID: "nope", Content: "orphan"}},
	}}, view.Cursor)

	// Assert
	assert.Equal(t, before+1, view.TotalTurns)
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
