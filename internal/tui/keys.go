package tui

// KeyMap is implemented per-screen so the help overlay can render the
// currently active keybindings.
//
// TODO(phase-6): add bubbles/key.Binding fields and help integration.
type KeyMap interface {
	ShortHelp() []string
	FullHelp() [][]string
}
