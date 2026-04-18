package tui_test

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func editorFixture() tui.Editor {
	sess := &parser.Session{Turns: []parser.Turn{
		{Role: parser.RoleUser}, {Role: parser.RoleAssistant},
		{Role: parser.RoleUser}, {Role: parser.RoleAssistant},
	}}
	ts := []topics.Topic{
		{Label: "A", TurnIndices: []int{0, 1}, TokenCount: 1000},
		{Label: "B", TurnIndices: []int{2, 3}, TokenCount: 2000},
	}
	return tui.NewEditor(sess, ts)
}

func TestEditor_ToggleSelection(t *testing.T) {
	e := editorFixture()
	u, _ := e.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, u.(tui.Editor).IsSelected(0))
}

func TestEditor_SelectAllAndNone(t *testing.T) {
	e := editorFixture()
	all, _ := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	require.True(t, all.(tui.Editor).IsSelected(0))
	require.True(t, all.(tui.Editor).IsSelected(1))
	none, _ := all.(tui.Editor).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	assert.False(t, none.(tui.Editor).IsSelected(0))
	assert.False(t, none.(tui.Editor).IsSelected(1))
}

func TestEditor_CursorMoves(t *testing.T) {
	e := editorFixture()
	u, _ := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, u.(tui.Editor).Cursor())
}

func TestEditor_EscEmitsReturnToOverview(t *testing.T) {
	e := editorFixture()
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	_, ok := cmd().(tui.ReturnToOverviewMsg)
	assert.True(t, ok)
}

func TestEditor_ViewIncludesAllTopicsAndFooter(t *testing.T) {
	e := editorFixture()
	e = e.SetSize(80, 20).(tui.Editor)
	u, _ := e.Update(tea.KeyMsg{Type: tea.KeySpace})
	out := u.(tui.Editor).View()
	assert.Contains(t, out, "A")
	assert.Contains(t, out, "B")
	assert.Contains(t, out, "1 topic")
}

func TestEditor_ViewShowsStatus(t *testing.T) {
	e := editorFixture()
	e = e.SetSize(80, 20).(tui.Editor)
	u2, _ := e.Update(tui.PruneErrMsg{Err: fmt.Errorf("locked")})
	assert.Contains(t, u2.(tui.Editor).View(), "locked")
}
