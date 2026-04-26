package tui

import "github.com/charmbracelet/bubbles/key"

// ScrollKeys are vim-style scroll bindings reused across scrollable views.
// Embedded into view-specific keymaps (Picker, Overview, Replay, ...) so the
// same set of keys produces identical motion semantics everywhere.
type ScrollKeys struct {
	Top      key.Binding // "g"      — jump to first row / top of scroll region
	Bottom   key.Binding // "G"      — jump to last row / bottom of scroll region
	PageDown key.Binding // "ctrl+d" — half-page down
	PageUp   key.Binding // "ctrl+u" — half-page up
}

// DefaultScrollKeys returns the standard vim scroll bindings.
func DefaultScrollKeys() ScrollKeys {
	return ScrollKeys{
		Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("^d", "page down")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("^u", "page up")),
	}
}

// PickerKeys is the session picker keymap. Extended by Phase 3+ when topic
// overview and replay mode register their own maps.
type PickerKeys struct {
	ScrollKeys
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	View    key.Binding
	Replay  key.Binding
	Delete  key.Binding
	Restore key.Binding
	Search  key.Binding
	Quit    key.Binding
}

// DefaultPickerKeys returns the picker bindings.
func DefaultPickerKeys() PickerKeys {
	return PickerKeys{
		ScrollKeys: DefaultScrollKeys(),
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Open:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		View:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view")),
		Replay:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
		Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Restore:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// OverviewKeys is the Topic Overview keymap (SPEC §3.2).
type OverviewKeys struct {
	ScrollKeys
	Up        key.Binding
	Down      key.Binding
	Expand    key.Binding
	FoldAll   key.Binding
	Select    key.Binding
	ToggleAll key.Binding
	Prune     key.Binding
	Replay    key.Binding
	Resume    key.Binding
	Stats     key.Binding
	Search    key.Binding
	Back      key.Binding
	Quit      key.Binding
}

// DefaultOverviewKeys returns the topic overview bindings.
func DefaultOverviewKeys() OverviewKeys {
	return OverviewKeys{
		ScrollKeys: DefaultScrollKeys(),
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Expand:     key.NewBinding(key.WithKeys("enter", "right", "l"), key.WithHelp("enter", "expand")),
		FoldAll:    key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fold all")),
		Select:     key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
		ToggleAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle all")),
		Prune:      key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prune")),
		Replay:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
		Resume:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "resume")),
		Stats:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "stats")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// ReplayKeys enumerates keybindings for the Replay screen (SPEC §3.3).
type ReplayKeys struct {
	ScrollKeys
	Next           key.Binding
	Prev           key.Binding
	AutoPlay       key.Binding
	NextTopic      key.Binding
	PrevTopic      key.Binding
	ToggleThinking key.Binding
	ToggleSlim     key.Binding
	SpeedUp        key.Binding
	SpeedDown      key.Binding
	Expand         key.Binding
	SidebarFocus   key.Binding
	Search         key.Binding
	Back           key.Binding
	Quit           key.Binding
}

func DefaultReplayKeys() ReplayKeys {
	return ReplayKeys{
		ScrollKeys:     DefaultScrollKeys(),
		Next:           key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next turn")),
		Prev:           key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "prev turn")),
		AutoPlay:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "auto-play")),
		NextTopic:      key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next topic")),
		PrevTopic:      key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev topic")),
		ToggleThinking: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "thinking")),
		ToggleSlim:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "slim")),
		SpeedUp:        key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "speed up")),
		SpeedDown:      key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "speed down")),
		Expand:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand tool")),
		SidebarFocus:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "sidebar")),
		Search:         key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Back:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Quit:           key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}
