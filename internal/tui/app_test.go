package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_ViewNoSessions_ShowsEmptyMessage(t *testing.T) {
	out := tui.NewApp(nil).View()
	assert.Contains(t, out, "No sessions found")
}

func TestApp_OpenSessionMsg_TransitionsToLoading(t *testing.T) {
	app := tui.NewApp([]parser.SessionMeta{{
		ID: "a", Path: "/p/a.jsonl", Project: "p", Source: parser.SourceClaude,
	}})
	next, cmd := app.Update(tui.OpenSessionMsg{Meta: parser.SessionMeta{Path: "/p/a.jsonl"}})
	a := next.(tui.App)
	assert.Contains(t, a.View(), "parsing")
	require.NotNil(t, cmd)
}

func TestApp_SessionLoadedMsg_TransitionsToOverview(t *testing.T) {
	app := tui.NewApp(nil)
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
	app := tui.NewApp([]parser.SessionMeta{{ID: "a", Project: "p"}})
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser, Content: "x", Tokens: 1}}}
	loaded := tui.SessionLoadedMsg{Session: sess, Topics: topics.Cluster(sess, topics.DefaultOptions())}
	a, _ := app.Update(loaded)
	next, _ := a.(tui.App).Update(tui.ReturnToPickerMsg{})
	assert.Contains(t, next.(tui.App).View(), "Sessions")
}

func TestApp_SessionLoadErrMsg_ShowsErrorState(t *testing.T) {
	app := tui.NewApp(nil)
	next, _ := app.Update(tui.SessionLoadErrMsg{Path: "/x", Err: errBoom})
	out := next.(tui.App).View()
	assert.Contains(t, out, "error:")
	assert.Contains(t, out, "press esc")
}

var errBoom = appTestErr("boom")

type appTestErr string

func (e appTestErr) Error() string { return string(e) }

func TestApp_QuitKey_StillQuitsInPicker(t *testing.T) {
	app := tui.NewApp(nil)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}
