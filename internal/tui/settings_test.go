package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestSettings_VimGotoTopBottom(t *testing.T) {
	// Arrange — start at cursor 0; press G to jump to last field.
	s := tui.NewSettings(config.Default(), 80, 24)

	// Act
	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})

	// Assert — cursor at last field. Cursor() not exported, so assert via
	// View output: the last field row should be the marked one.
	view := next.View()
	assert.Contains(t, view, "▸ Gap threshold")
}

func TestSettings_VimPageKeys_Bounds(t *testing.T) {
	// Arrange
	s := tui.NewSettings(config.Default(), 80, 24)

	// Act — ctrl+d should jump to last on this small list, ctrl+u back.
	next, _, _ := s.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	next2, _, _ := next.Update(tea.KeyMsg{Type: tea.KeyCtrlU})

	// Assert — ctrl+u brought us back to top (Theme field marked).
	view := next2.View()
	assert.Contains(t, view, "▸ Theme")
}
