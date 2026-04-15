package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApp_QuitKey_ReturnsQuitCmd(t *testing.T) {
	// Arrange
	app := tui.NewApp()

	// Act
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok)
}

func TestApp_View_ContainsTitle(t *testing.T) {
	// Arrange
	app := tui.NewApp()

	// Act
	out := app.View()

	// Assert
	assert.Contains(t, out, "AgentLens")
}
