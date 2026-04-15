package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtures() []parser.SessionMeta {
	return []parser.SessionMeta{
		{ID: "a", Path: "/p/a.jsonl", Project: "proj-a", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Path: "/p/b.jsonl", Project: "proj-b", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-24 * time.Hour)},
		{ID: "c", Path: "/p/c.jsonl", Project: "proj-c", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-72 * time.Hour)},
	}
}

func TestPicker_DownKey_MovesCursor(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 1, p.Cursor())
}

func TestPicker_UpKey_AtTopStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 0, p.Cursor())
}

func TestPicker_DownKey_AtBottomStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())
	for i := 0; i < 5; i++ {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = m2.(tui.Picker)
	}

	// Assert — clamped at len-1
	assert.Equal(t, 2, m.Cursor())
}

func TestPicker_QuitKey_EmitsQuitCmd(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestPicker_Empty_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	m := tui.NewPicker(nil)

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}

func TestPicker_View_ContainsProjectName(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "proj-a")
}
