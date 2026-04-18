package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshly/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestConfirm_YKey_Confirms(t *testing.T) {
	// Arrange
	c := tui.NewConfirm("delete?", "really?")

	// Act
	next, _ := c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	// Assert
	cc := next.(tui.Confirm)
	assert.True(t, cc.Done())
	assert.True(t, cc.Confirmed())
}

func TestConfirm_NKey_Cancels(t *testing.T) {
	// Arrange
	c := tui.NewConfirm("delete?", "really?")

	// Act
	next, _ := c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	// Assert
	cc := next.(tui.Confirm)
	assert.True(t, cc.Done())
	assert.False(t, cc.Confirmed())
}

func TestConfirm_EscKey_Cancels(t *testing.T) {
	// Arrange
	c := tui.NewConfirm("delete?", "really?")

	// Act
	next, _ := c.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Assert
	cc := next.(tui.Confirm)
	assert.True(t, cc.Done())
	assert.False(t, cc.Confirmed())
}

func TestConfirm_View_ContainsTitleAndBody(t *testing.T) {
	// Arrange
	c := tui.NewConfirm("Delete session", "This is permanent.")

	// Act
	out := c.View()

	// Assert
	assert.Contains(t, out, "Delete session")
	assert.Contains(t, out, "This is permanent.")
	assert.Contains(t, out, "y/n")
}
