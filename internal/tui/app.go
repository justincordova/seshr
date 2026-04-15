package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
)

// App is the root Bubbletea model. For Phase 2 the app is a thin wrapper
// around Picker — Phase 3 adds the Topic Overview as a second screen.
type App struct {
	picker Picker
}

// NewApp returns the root model with a pre-populated session list.
func NewApp(metas []parser.SessionMeta) App {
	return App{picker: NewPicker(metas)}
}

// Init satisfies tea.Model.
func (a App) Init() tea.Cmd { return a.picker.Init() }

// Update satisfies tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.picker.Update(msg)
	a.picker = m.(Picker)
	return a, cmd
}

// View satisfies tea.Model.
func (a App) View() string { return a.picker.View() }
