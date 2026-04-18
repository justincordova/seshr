package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshly/internal/config"
	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/topics"
	"github.com/justincordova/seshly/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCfg returns a default config for use in tests.
func testCfg() config.Config { return config.Default() }

func TestApp_ViewNoSessions_ShowsEmptyMessage(t *testing.T) {
	out := tui.NewApp(nil, testCfg()).View()
	assert.Contains(t, out, "No sessions found")
}

func TestApp_OpenSessionMsg_TransitionsToLoading(t *testing.T) {
	app := tui.NewApp([]parser.SessionMeta{{
		ID: "a", Path: "/p/a.jsonl", Project: "p", Source: parser.SourceClaude,
	}}, testCfg())
	next, cmd := app.Update(tui.OpenSessionMsg{Meta: parser.SessionMeta{Path: "/p/a.jsonl"}})
	a := next.(tui.App)
	assert.Contains(t, a.View(), "parsing")
	require.NotNil(t, cmd)
}

func TestApp_SessionLoadedMsg_TransitionsToOverview(t *testing.T) {
	app := tui.NewApp(nil, testCfg())
	sess := &parser.Session{
		ID:         "x",
		Source:     parser.SourceClaude,
		CreatedAt:  time.Unix(0, 0),
		ModifiedAt: time.Unix(60, 0),
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Timestamp: time.Unix(0, 0), Content: "hi", Tokens: 3},
			{Role: parser.RoleAssistant, Timestamp: time.Unix(1, 0), Content: "yo", Tokens: 2},
		},
		TokenCount: 5,
	}
	tops := topics.Cluster(sess, topics.DefaultOptions())
	next, _ := app.Update(tui.SessionLoadedMsg{Session: sess, Topics: tops})
	a := next.(tui.App)
	out := a.View()
	assert.NotContains(t, out, "parsing")
	assert.Contains(t, out, string(parser.SourceClaude))
}

func TestApp_ReturnToPickerMsg_GoesBack(t *testing.T) {
	app := tui.NewApp([]parser.SessionMeta{{ID: "a", Project: "p"}}, testCfg())
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "x", Tokens: 1}}}
	loaded := tui.SessionLoadedMsg{Session: sess, Topics: topics.Cluster(sess, topics.DefaultOptions())}
	a, _ := app.Update(loaded)
	next, _ := a.(tui.App).Update(tui.ReturnToPickerMsg{})
	assert.Contains(t, next.(tui.App).View(), "Sessions")
}

func TestApp_SessionLoadErrMsg_ShowsErrorState(t *testing.T) {
	app := tui.NewApp(nil, testCfg())
	next, _ := app.Update(tui.SessionLoadErrMsg{Path: "/x", Err: errBoom})
	out := next.(tui.App).View()
	assert.Contains(t, out, "error:")
	assert.Contains(t, out, "press esc")
}

func TestApp_QuitKey_QuitsInErrorState(t *testing.T) {
	// Arrange
	app := tui.NewApp(nil, testCfg())
	next, _ := app.Update(tui.SessionLoadErrMsg{Path: "/x", Err: errTest})
	errApp := next.(tui.App)

	// Act
	_, cmd := errApp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

var errBoom = appTestErr("boom")
var errTest = appTestErr("test")

type appTestErr string

func (e appTestErr) Error() string { return string(e) }

func TestApp_QuitKey_StillQuitsInPicker(t *testing.T) {
	app := tui.NewApp(nil, testCfg())
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestApp_OpenReplayTransitionsToReplayState(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "hi"}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	app := tui.AppInOverview(sess, ts)
	next, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a := next.(tui.App)

	a2, _ := a.Update(tui.OpenReplayMsg{})

	assert.Equal(t, tui.StateReplay, a2.(tui.App).State())
}

func TestApp_ReturnToOverviewFromReplay(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "hi"}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	app := tui.AppInOverview(sess, ts)
	next, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a := next.(tui.App)
	a2, _ := a.Update(tui.OpenReplayMsg{})
	require.Equal(t, tui.StateReplay, a2.(tui.App).State())

	a3, _ := a2.(tui.App).Update(tui.ReturnToOverviewMsg{})

	assert.Equal(t, tui.StateOverview, a3.(tui.App).State())
}

func TestApp_OpenEditorTransitionsToEditorState(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "hi"}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	app := tui.AppInOverview(sess, ts)
	next, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a := next.(tui.App)

	a2, _ := a.Update(tui.OpenEditorMsg{})

	assert.Equal(t, tui.StateEditor, a2.(tui.App).State())
}

func TestApp_EditorEscReturnsToOverview(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "hi"}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	app := tui.AppInOverview(sess, ts)
	next, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a := next.(tui.App)
	a2, _ := a.Update(tui.OpenEditorMsg{})
	require.Equal(t, tui.StateEditor, a2.(tui.App).State())

	a3, _ := a2.(tui.App).Update(tui.ReturnToOverviewMsg{})

	assert.Equal(t, tui.StateOverview, a3.(tui.App).State())
}

func TestApp_RestoreRequestedShowsConfirm(t *testing.T) {
	app := tui.NewApp([]parser.SessionMeta{{ID: "a", Path: "/x/a.jsonl", HasBackup: true}}, testCfg())
	next, _ := app.Update(tui.RestoreRequestedMsg{Path: "/x/a.jsonl"})
	assert.Equal(t, tui.StateConfirmRestore, next.(tui.App).State())
}

func TestApp_RestoreDoneReturnsToList(t *testing.T) {
	app := tui.NewApp([]parser.SessionMeta{{ID: "a", Path: "/x/a.jsonl", HasBackup: true}}, testCfg())
	a2, _ := app.Update(tui.RestoreRequestedMsg{Path: "/x/a.jsonl"})
	a3, _ := a2.(tui.App).Update(tui.RestoreDoneMsg{Path: "/x/a.jsonl"})
	assert.Equal(t, tui.StateList, a3.(tui.App).State())
}
