package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_QuitKey_ReturnsQuitCmd(t *testing.T) {
	// Arrange
	app := tui.NewApp(nil)

	// Act
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestApp_ViewWithSessions_DelegatesToPicker(t *testing.T) {
	// Arrange
	app := tui.NewApp([]parser.SessionMeta{{
		ID: "abc", Path: "/p/abc.jsonl", Project: "demo", Source: parser.SourceClaude,
	}})

	// Act
	out := app.View()

	// Assert
	assert.Contains(t, out, "demo")
}

func TestApp_ViewNoSessions_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	app := tui.NewApp(nil)

	// Act
	out := app.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}
