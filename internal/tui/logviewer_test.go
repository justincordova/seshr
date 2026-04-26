package tui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/tui"
)

// TestLogViewer_VimScrollKeysNoCrash verifies vim bindings are accepted
// and don't crash. The actual scrolling effect is on a bubbles viewport
// which is exercised by the bubbles library's own tests.
func TestLogViewer_VimScrollKeysNoCrash(t *testing.T) {
	lv := tui.NewLogViewer(80, 24)

	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'g'}},
		{Type: tea.KeyRunes, Runes: []rune{'G'}},
		{Type: tea.KeyCtrlD},
		{Type: tea.KeyCtrlU},
	} {
		var done bool
		lv, done = lv.Update(k)
		if done {
			t.Fatalf("vim scroll key %v should not close the log viewer", k)
		}
	}
}
