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

// OverviewKeys is the Topic Overview keymap (SPEC §3.2).
type OverviewKeys struct {
	Up     key.Binding
	Down   key.Binding
	Expand key.Binding
	Replay key.Binding
	Edit   key.Binding
	Stats  key.Binding
	Back   key.Binding
	Quit   key.Binding
}

// DefaultOverviewKeys returns the v1 topic overview bindings per SPEC §3.2.
func DefaultOverviewKeys() OverviewKeys {
	return OverviewKeys{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Expand: key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("enter", "expand")),
		Replay: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
		Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Stats:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "stats")),
		Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ReplayKeys enumerates keybindings for the Replay screen (SPEC §3.3).
// Speed keys 1-9 are handled by a range check in Update, not stored here.
type ReplayKeys struct {
	Next           key.Binding
	Prev           key.Binding
	AutoPlay       key.Binding
	NextTopic      key.Binding
	PrevTopic      key.Binding
	ToggleThinking key.Binding
	ToggleWrap     key.Binding
	Expand         key.Binding
	Back           key.Binding
	Quit           key.Binding
}

func DefaultReplayKeys() ReplayKeys {
	return ReplayKeys{
		Next:           key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next turn")),
		Prev:           key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev turn")),
		AutoPlay:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "auto-play")),
		NextTopic:      key.NewBinding(key.WithKeys("]", "n"), key.WithHelp("]/n", "next topic")),
		PrevTopic:      key.NewBinding(key.WithKeys("[", "p"), key.WithHelp("[/p", "prev topic")),
		ToggleThinking: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "thinking")),
		ToggleWrap:     key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "wrap")),
		Expand:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand tool")),
		Back:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:           key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

type EditorKeys struct {
	Up         key.Binding
	Down       key.Binding
	Toggle     key.Binding
	SelectAll  key.Binding
	SelectNone key.Binding
	Prune      key.Binding
	Expand     key.Binding
	Cancel     key.Binding
	Quit       key.Binding
}

func DefaultEditorKeys() EditorKeys {
	return EditorKeys{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Toggle:     key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		SelectAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all")),
		SelectNone: key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "none")),
		Prune:      key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prune")),
		Expand:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand")),
		Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
