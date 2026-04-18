package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/editor"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
)

type appState int

const (
	stateList appState = iota
	stateLoading
	stateOverview
	stateError
	stateReplay
	stateEditor
	stateConfirmRestore
)

// Exported state name constants for use in tests.
const (
	StateList           = "list"
	StateLoading        = "loading"
	StateOverview       = "overview"
	StateError          = "error"
	StateReplay         = "replay"
	StateEditor         = "editor"
	StateConfirmRestore = "confirm_restore"
)

// App is the root Bubbletea model. Routes between picker, loading, overview, and replay.
type App struct {
	state        appState
	picker       Picker
	overview     Overview
	replay       Replay
	editorModel  Editor
	spinner      spinner.Model
	loading      string
	lastErr      string
	styles       Styles
	width        int
	height       int
	session      *parser.Session
	topicsCache  []topics.Topic
	restorePath  string
	restoreModal Confirm
}

// State returns a string name for the current state, usable in tests.
func (a App) State() string {
	switch a.state {
	case stateList:
		return StateList
	case stateLoading:
		return StateLoading
	case stateOverview:
		return StateOverview
	case stateReplay:
		return StateReplay
	case stateEditor:
		return StateEditor
	case stateConfirmRestore:
		return StateConfirmRestore
	case stateError:
		return StateError
	default:
		return "unknown"
	}
}

// AppInOverview returns an App pre-seeded in stateOverview, useful for tests.
func AppInOverview(sess *parser.Session, ts []topics.Topic) App {
	th := CatppuccinMocha()
	return App{
		state:       stateOverview,
		session:     sess,
		topicsCache: ts,
		overview:    NewOverview(sess, ts),
		styles:      NewStyles(th),
	}
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
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = wsm.Width
		a.height = wsm.Height
	}

	switch m := msg.(type) {
	case OpenSessionMsg:
		a.state = stateLoading
		a.loading = m.Meta.Path
		return a, tea.Batch(a.spinner.Tick, LoadSessionCmd(m.Meta.Path))
	case SessionLoadedMsg:
		a.session = m.Session
		a.topicsCache = m.Topics
		a.overview = NewOverview(m.Session, m.Topics)
		if a.width > 0 {
			om, _ := a.overview.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.overview = om.(Overview)
		}
		a.state = stateOverview
		return a, nil
	case SessionLoadErrMsg:
		a.state = stateError
		a.lastErr = fmt.Sprintf("load %s: %v", m.Path, m.Err)
		return a, nil
	case ReturnToPickerMsg:
		a.state = stateList
		return a, nil
	case OpenReplayMsg:
		a.replay = NewReplay(a.session, a.topicsCache)
		a.replay = a.replay.SetSize(a.width, a.height).(Replay)
		a.state = stateReplay
		return a, a.replay.Init()
	case ReturnToOverviewMsg:
		a.state = stateOverview
		return a, nil
	case OpenEditorMsg:
		a.editorModel = NewEditor(a.session, a.topicsCache)
		a.editorModel = a.editorModel.SetSize(a.width, a.height).(Editor)
		a.state = stateEditor
		return a, a.editorModel.Init()
	case RestoreRequestedMsg:
		a.restorePath = m.Path
		a.restoreModal = NewConfirm("Restore from backup?", "This will overwrite the current session file with the backup.")
		a.state = stateConfirmRestore
		return a, nil
	case RestoreDoneMsg:
		a.state = stateList
		return a, nil
	case RestoreErrMsg:
		a.lastErr = m.Err.Error()
		a.state = stateError
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
	case stateReplay:
		rm, cmd := a.replay.Update(msg)
		a.replay = rm.(Replay)
		return a, cmd
	case stateEditor:
		em, cmd := a.editorModel.Update(msg)
		a.editorModel = em.(Editor)
		return a, cmd
	case stateConfirmRestore:
		if km, ok := msg.(tea.KeyMsg); ok {
			m, _ := a.restoreModal.Update(km)
			c := m.(Confirm)
			a.restoreModal = c
			if c.Done() {
				if c.Confirmed() {
					return a, restoreCmd(a.restorePath)
				}
				a.state = stateList
			}
			return a, nil
		}
		return a, nil
	case stateError:
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				a.state = stateList
				return a, nil
			case "q":
				return a, tea.Quit
			}
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
	case stateReplay:
		return a.replay.View()
	case stateEditor:
		return a.editorModel.View()
	case stateConfirmRestore:
		return a.restoreModal.View()
	case stateError:
		return a.styles.App.Render(
			a.styles.Error.Render("error: ") + a.lastErr + "\n\n" +
				a.styles.Hint.Render("press esc to go back"),
		)
	default:
		return a.picker.View()
	}
}

type RestoreDoneMsg struct{ Path string }
type RestoreErrMsg struct{ Err error }

func restoreCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := editor.Restore(path); err != nil {
			return RestoreErrMsg{Err: err}
		}
		return RestoreDoneMsg{Path: path}
	}
}
