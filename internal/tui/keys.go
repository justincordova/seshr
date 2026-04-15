package tui

import "github.com/charmbracelet/bubbles/key"

// PickerKeys is the session picker keymap. Extended by Phase 3+ when topic
// overview and replay mode register their own maps.
type PickerKeys struct {
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Delete  key.Binding
	Restore key.Binding
	Quit    key.Binding
}

// DefaultPickerKeys returns the v1 picker bindings per SPEC §3.1.
func DefaultPickerKeys() PickerKeys {
	return PickerKeys{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Delete:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Restore: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
