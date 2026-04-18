package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/config"
	"github.com/justincordova/agentlens/internal/editor"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
)

// currentBindings returns the keybindings for the currently active screen,
// used to populate the help overlay.
func (a App) currentBindings() []key.Binding {
	switch a.state {
	case stateList:
		k := DefaultPickerKeys()
		return []key.Binding{k.Up, k.Down, k.Open, k.Replay, k.Edit, k.Delete, k.Restore, k.Search, k.Quit}
	case stateOverview:
		k := DefaultOverviewKeys()
		return []key.Binding{k.Up, k.Down, k.Expand, k.Replay, k.Edit, k.Stats, k.Search, k.Back, k.Quit}
	case stateReplay:
		k := DefaultReplayKeys()
		return []key.Binding{k.Next, k.Prev, k.AutoPlay, k.NextTopic, k.PrevTopic, k.ToggleThinking, k.ToggleWrap, k.Expand, k.SidebarFocus, k.Search, k.Back, k.Quit}
	case stateEditor:
		k := DefaultEditorKeys()
		return []key.Binding{k.Up, k.Down, k.Toggle, k.SelectAll, k.SelectNone, k.Prune, k.Expand, k.Cancel, k.Quit}
	default:
		return nil
	}
}

const (
	minWidth  = 60
	minHeight = 15
)

// overlayKind identifies which overlay (if any) is active.
type overlayKind int

const (
	ovNone     overlayKind = iota
	ovHelp                 // ? — keybinding reference
	ovLogView              // L — debug log viewer
	ovSettings             // , — settings popup
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
	theme        Theme
	cfg          config.Config
	width        int
	height       int
	session      *parser.Session
	topicsCache  []topics.Topic
	restorePath  string
	restoreModal Confirm
	prevState    appState
	autoReplay   bool
	autoEdit     bool
	// overlay fields
	overlay  overlayKind
	help     Help
	logView  LogViewer
	settings Settings
}

// overlayActive reports whether any overlay is currently shown.
func (a App) overlayActive() bool { return a.overlay != ovNone }

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
		theme:       th,
		cfg:         config.Default(),
	}
}

// NewApp returns the root model with a pre-populated session list.
// cfg is the loaded user configuration; pass config.Default() in tests.
func NewApp(metas []parser.SessionMeta, cfg config.Config) App {
	th := ThemeByName(cfg.Theme)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return App{
		state:   stateList,
		picker:  NewPicker(metas, th),
		spinner: sp,
		styles:  NewStyles(th),
		theme:   th,
		cfg:     cfg,
	}
}

func (a App) Init() tea.Cmd { return a.picker.Init() }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// ── Window resize: always propagate ──────────────────────────────────────
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = wsm.Width
		a.height = wsm.Height
		if a.overlay == ovLogView {
			a.logView = a.logView.SetSize(wsm.Width, wsm.Height)
		}
		if a.overlay == ovHelp {
			a.help = a.help.SetSize(wsm.Width, wsm.Height)
		}
		if a.overlay == ovSettings {
			a.settings = a.settings.SetSize(wsm.Width, wsm.Height)
		}
	}

	// ── Active overlay: route all input to it ────────────────────────────────
	if a.overlayActive() {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch a.overlay {
			case ovHelp:
				// Any key closes help.
				_ = km
				a.overlay = ovNone
				return a, nil
			case ovLogView:
				var done bool
				a.logView, done = a.logView.Update(msg)
				if done {
					a.overlay = ovNone
				}
				return a, nil
			case ovSettings:
				var done bool
				var cmd tea.Cmd
				a.settings, done, cmd = a.settings.Update(msg)
				if done {
					a.overlay = ovNone
				}
				return a, cmd
			}
		}
		return a, nil
	}

	// ── Global key intercepts (active when no overlay is open) ───────────────
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "?":
			a.help = NewHelp(a.currentBindings(), a.width, a.height)
			a.overlay = ovHelp
			return a, nil
		case "L":
			a.logView = NewLogViewer(a.width, a.height)
			a.overlay = ovLogView
			return a, nil
		case ",":
			a.settings = NewSettings(a.cfg, a.width, a.height)
			a.overlay = ovSettings
			return a, nil
		}
	}

	// ── SettingsSavedMsg: rebuild theme/styles ───────────────────────────────
	if sm, ok := msg.(SettingsSavedMsg); ok {
		a.cfg = sm.Cfg
		a.theme = ThemeByName(sm.Cfg.Theme)
		a.styles = NewStyles(a.theme)
		return a, nil
	}

	switch m := msg.(type) {
	case OpenSessionMsg:
		a.state = stateLoading
		a.loading = m.Meta.Path
		return a, tea.Batch(a.spinner.Tick, LoadSessionCmd(m.Meta.Path))
	case OpenSessionAndReplayMsg:
		a.state = stateLoading
		a.loading = m.Meta.Path
		a.autoReplay = true
		return a, tea.Batch(a.spinner.Tick, LoadSessionCmd(m.Meta.Path))
	case OpenSessionAndEditMsg:
		a.state = stateLoading
		a.loading = m.Meta.Path
		a.autoEdit = true
		return a, tea.Batch(a.spinner.Tick, LoadSessionCmd(m.Meta.Path))
	case SessionLoadedMsg:
		a.session = m.Session
		a.topicsCache = m.Topics
		a.overview = NewOverview(m.Session, m.Topics)
		if a.width > 0 {
			om, _ := a.overview.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.overview = om.(Overview)
		}
		if a.autoReplay {
			a.autoReplay = false
			a.replay = NewReplay(m.Session, m.Topics)
			a.replay = a.replay.SetSize(a.width, a.height).(Replay)
			a.state = stateReplay
			return a, a.replay.Init()
		}
		if a.autoEdit {
			a.autoEdit = false
			a.editorModel = NewEditor(m.Session, m.Topics)
			a.editorModel = a.editorModel.SetSize(a.width, a.height).(Editor)
			a.state = stateEditor
			return a, a.editorModel.Init()
		}
		a.state = stateOverview
		return a, nil
	case SessionLoadErrMsg:
		a.prevState = a.state
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
		a.overview = NewOverview(a.session, a.topicsCache)
		a.state = stateList
		return a, rescanCmd()
	case RestoreErrMsg:
		a.lastErr = m.Err.Error()
		a.prevState = a.state
		a.state = stateError
		return a, nil
	case RescanDoneMsg:
		if m.Metas != nil {
			a.picker = NewPicker(m.Metas, a.theme)
		}
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
			case "esc", "enter":
				if a.prevState != 0 {
					a.state = a.prevState
					a.prevState = 0
				} else {
					a.state = stateList
				}
				return a, nil
			case "q":
				return a, tea.Quit
			}
		}
	}
	return a, nil
}

func (a App) View() string {
	if a.width > 0 && a.height > 0 && (a.width < minWidth || a.height < minHeight) {
		return a.styles.App.Render(
			fmt.Sprintf("Terminal too small (%dx%d). Need at least %dx%d.", a.width, a.height, minWidth, minHeight),
		)
	}

	// Log viewer replaces the base screen entirely.
	if a.overlay == ovLogView {
		return a.logView.View()
	}

	// Render the base screen first.
	var base string
	switch a.state {
	case stateLoading:
		base = a.styles.App.Render(fmt.Sprintf("%s  parsing %s…\n", a.spinner.View(), a.loading))
	case stateOverview:
		base = a.overview.View()
	case stateReplay:
		base = a.replay.View()
	case stateEditor:
		base = a.editorModel.View()
	case stateConfirmRestore:
		base = a.restoreModal.View()
	case stateError:
		base = a.styles.App.Render(
			a.styles.Error.Render("error: ") + a.lastErr + "\n\n" +
				a.styles.Hint.Render("press esc to go back"),
		)
	default:
		base = a.picker.View()
	}

	// Layer overlay on top.
	switch a.overlay {
	case ovHelp:
		return a.help.View()
	case ovSettings:
		return a.settings.View()
	}
	return base
}

type RestoreDoneMsg struct{ Path string }
type RestoreErrMsg struct{ Err error }
type RescanDoneMsg struct {
	Metas []parser.SessionMeta
}

func restoreCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := editor.Restore(path); err != nil {
			return RestoreErrMsg{Err: err}
		}
		return RestoreDoneMsg{Path: path}
	}
}

func rescanCmd() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return RescanDoneMsg{}
		}
		metas, _ := parser.Scan(filepath.Join(home, ".claude", "projects"))
		return RescanDoneMsg{Metas: metas}
	}
}
