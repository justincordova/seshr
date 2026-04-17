package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
)

type appState int

const (
	stateList appState = iota
	stateLoading
	stateOverview
	stateError
)

// App is the root Bubbletea model. Routes between picker, loading, and overview.
type App struct {
	state    appState
	picker   Picker
	overview Overview
	spinner  spinner.Model
	loading  string
	lastErr  string
	styles   Styles
}

// NewApp returns the root model with a pre-populated session list.
func NewApp(metas []parser.SessionMeta) App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return App{
		state:   stateList,
		picker:  NewPicker(metas),
		spinner: sp,
		styles:  NewStyles(CatppuccinMocha()),
	}
}

func (a App) Init() tea.Cmd { return a.picker.Init() }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case OpenSessionMsg:
		a.state = stateLoading
		a.loading = m.Meta.Path
		return a, tea.Batch(a.spinner.Tick, LoadSessionCmd(m.Meta.Path))
	case SessionLoadedMsg:
		a.overview = NewOverview(m.Session, m.Topics)
		a.state = stateOverview
		return a, nil
	case SessionLoadErrMsg:
		a.state = stateError
		a.lastErr = fmt.Sprintf("load %s: %v", m.Path, m.Err)
		return a, nil
	case ReturnToPickerMsg:
		a.state = stateList
		return a, nil
	case spinner.TickMsg:
		if a.state == stateLoading {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(m)
			return a, cmd
		}
		return a, nil
	}

	switch a.state {
	case stateList:
		pm, cmd := a.picker.Update(msg)
		a.picker = pm.(Picker)
		return a, cmd
	case stateOverview:
		om, cmd := a.overview.Update(msg)
		a.overview = om.(Overview)
		return a, cmd
	case stateError:
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			a.state = stateList
			return a, nil
		}
	}
	return a, nil
}

func (a App) View() string {
	switch a.state {
	case stateLoading:
		return a.styles.App.Render(fmt.Sprintf("%s  parsing %s…\n", a.spinner.View(), a.loading))
	case stateOverview:
		return a.overview.View()
	case stateError:
		return a.styles.App.Render(
			a.styles.Error.Render("error: ") + a.lastErr + "\n\n" +
				a.styles.Hint.Render("press esc to go back"),
		)
	default:
		return a.picker.View()
	}
}
