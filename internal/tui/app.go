package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// App is the root Bubbletea model.
//
// TODO(phase-2+): replace with a router that switches between screens.
type App struct {
	styles Styles
	quit   bool
}

// NewApp returns the placeholder root model.
func NewApp() App {
	return App{styles: NewStyles(CatppuccinMocha())}
}

// Init satisfies tea.Model.
func (a App) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "q", "ctrl+c", "esc":
			a.quit = true
			return a, tea.Quit
		}
	}
	return a, nil
}

// View satisfies tea.Model.
func (a App) View() string {
	if a.quit {
		return ""
	}
	title := a.styles.Title.Render("AgentLens")
	hint := a.styles.Hint.Render("press q to quit")
	return a.styles.App.Render(title + "\n" + hint + "\n")
}
