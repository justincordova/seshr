package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/editor"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
)

// currentBindings returns the keybindings for the currently active screen,
// used to populate the help overlay.
func (a App) currentBindings() []key.Binding {
	switch a.state {
	case stateList:
		k := DefaultPickerKeys()
		return []key.Binding{k.Up, k.Down, k.Open, k.Replay, k.Delete, k.Restore, k.Search, k.Quit}
	case stateLanding:
		k := DefaultLandingKeys()
		return []key.Binding{k.Topics, k.Replay, k.Resume, k.LivePicker, k.Search, k.Back, k.Quit}
	case stateOverview:
		k := DefaultOverviewKeys()
		return []key.Binding{k.Up, k.Down, k.Expand, k.FoldAll, k.Select, k.ToggleAll, k.Prune, k.Replay, k.Resume, k.Stats, k.Search, k.Back, k.Quit}
	case stateReplay:
		k := DefaultReplayKeys()
		return []key.Binding{k.Next, k.Prev, k.AutoPlay, k.NextTopic, k.PrevTopic, k.ToggleThinking, k.SpeedUp, k.SpeedDown, k.Expand, k.SidebarFocus, k.Search, k.Back, k.Quit}
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
	ovResume               // c — resume command
	ovInfo                 // i — session info
)

type appState int

const (
	stateList appState = iota
	stateLoading
	stateLanding // landing page (Phase 7)
	stateOverview
	stateError
	stateReplay
	stateConfirmRestore
)

// Exported state name constants for use in tests.
const (
	StateList           = "list"
	StateLoading        = "loading"
	StateLanding        = "landing"
	StateOverview       = "overview"
	StateError          = "error"
	StateReplay         = "replay"
	StateConfirmRestore = "confirm_restore"
)

// App is the root Bubbletea model. Routes between picker, loading, overview, and replay.
type App struct {
	state        appState
	picker       Picker
	overview     Overview
	replay       Replay
	spinner      spinner.Model
	loading      string
	lastErr      string
	styles       Styles
	theme        Theme
	cfg          config.Config
	width        int
	height       int
	session      *session.Session
	topicsCache  []topics.Topic
	restorePath  string
	restoreID    string
	restoreKind  session.SourceKind
	restoreModal Confirm
	prevState    appState
	autoReplay   bool
	scanRoot     string
	overlay      overlayKind
	help         Help
	logView      LogViewer
	settings     Settings
	registry     *backend.Registry
	scanner      *backend.ProcessScanner
	LiveDisabled bool

	// Phase 7: landing page and resume overlay.
	landing         LandingModel
	resumeOverlay   ResumeOverlayModel
	currentView     *SessionView
	currentViewMeta backend.SessionMeta

	// Live detection state (Phase 6).
	liveIndex     *LiveIndex
	scanFailCount int
	lastScanErr   error
	fastActive    bool // true when the fast ticker is running
	ctx           context.Context
	cancel        context.CancelFunc
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
	case stateLanding:
		return StateLanding
	case stateOverview:
		return StateOverview
	case stateReplay:
		return StateReplay
	case stateConfirmRestore:
		return StateConfirmRestore
	case stateError:
		return StateError
	default:
		return "unknown"
	}
}

// AppInOverview returns an App pre-seeded in stateOverview, useful for tests.
func AppInOverview(sess *session.Session, ts []topics.Topic) App {
	th := CatppuccinMocha()
	cfg := config.Default()
	return App{
		state:       stateOverview,
		session:     sess,
		topicsCache: ts,
		overview:    NewOverview(sess, ts, th, cfg.GapThresholdSeconds, nil),
		styles:      NewStyles(th),
		theme:       th,
		cfg:         cfg,
	}
}

// NewApp returns the root model with a pre-populated session list.
// cfg is the loaded user configuration; pass config.Default() in tests.
// reg may be nil in tests that don't exercise live detection or store access.
// noLive disables live detection if true.
func NewApp(metas []backend.SessionMeta, cfg config.Config, scanRoot string, reg *backend.Registry, noLive bool) App {
	th := ThemeByName(cfg.Theme)
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	ctx, cancel := context.WithCancel(context.Background())
	picker := NewPicker(metas, th, reg, cfg.PickerViewMode)
	return App{
		state:        stateList,
		picker:       picker,
		spinner:      sp,
		styles:       NewStyles(th),
		theme:        th,
		cfg:          cfg,
		scanRoot:     scanRoot,
		registry:     reg,
		scanner:      backend.NewProcessScanner(),
		LiveDisabled: noLive,
		liveIndex:    NewLiveIndex(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.picker.Init()}
	if !a.LiveDisabled {
		// Fire an immediate first detection so live sessions appear on
		// launch instead of after the first 10s tick. Then continue with
		// the periodic slow ticker.
		cmds = append(cmds,
			func() tea.Msg { return liveSlowMsg{At: time.Now()} },
		)
	}
	return tea.Batch(cmds...)
}

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
		if a.state == stateLanding {
			lm, _ := a.landing.Update(wsm)
			a.landing = lm.(LandingModel)
		}
		if a.overlay == ovResume {
			rm, _ := a.resumeOverlay.Update(wsm)
			a.resumeOverlay = rm.(ResumeOverlayModel)
		}
	}

	// ── Overlay close messages: handle before the overlay-active gate so the
	// gate doesn't swallow them. ─────────────────────────────────────────────
	if _, ok := msg.(CloseResumeOverlayMsg); ok {
		a.overlay = ovNone
		return a, nil
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
			case ovResume:
				nm, cmd := a.resumeOverlay.Update(msg)
				a.resumeOverlay = nm.(ResumeOverlayModel)
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

	// ── PickerViewModeChangedMsg: persist new view-mode preference ───────────
	if vm, ok := msg.(PickerViewModeChangedMsg); ok {
		a.cfg.PickerViewMode = vm.Mode
		if err := config.Save(a.cfg); err != nil {
			slog.Warn("save picker view mode failed", "err", err)
		}
		return a, nil
	}

	// ── Quit: cancel the app context ─────────────────────────────────────────
	if _, ok := msg.(tea.QuitMsg); ok {
		if a.cancel != nil {
			a.cancel()
		}
		return a, nil
	}

	// ── Slow tick (10s): run detectors, reconcile live index ─────────────────
	if stm, ok := msg.(liveSlowMsg); ok {
		return a.handleSlowTick(stm.At)
	}

	// ── Fast tick (2s): incremental load for live sessions ───────────────────
	if _, ok := msg.(liveFastMsg); ok {
		return a.handleFastTick()
	}

	switch m := msg.(type) {
	case OpenSessionMsg:
		a.currentViewMeta = m.Meta
		a.state = stateLoading
		a.loading = m.Meta.ID
		return a, tea.Batch(a.spinner.Tick, LoadSessionByIDCmd(m.Meta, a.registry, a.cfg.GapThresholdSeconds))
	case OpenSessionAndReplayMsg:
		a.state = stateLoading
		a.loading = m.Meta.ID
		a.autoReplay = true
		return a, tea.Batch(a.spinner.Tick, LoadSessionByIDCmd(m.Meta, a.registry, a.cfg.GapThresholdSeconds))
	case SessionLoadedMsg:
		a.session = m.Session
		a.topicsCache = m.Topics
		a.overview = NewOverview(m.Session, m.Topics, a.theme, a.cfg.GapThresholdSeconds, a.registry)
		if a.width > 0 {
			om, _ := a.overview.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.overview = om.(Overview)
		}
		if a.autoReplay {
			a.autoReplay = false
			a.replay = NewReplay(m.Session, m.Topics, a.theme)
			a.replay = a.replay.SetSize(a.width, a.height).(Replay)
			a.state = stateReplay
			return a, a.replay.Init()
		}
		// Build a SessionView. Live sessions go to the landing/cockpit;
		// ended sessions skip directly to Topic Overview (resume is also
		// reachable from there via `c`).
		if a.registry != nil {
			if _, ok := a.registry.Store(m.Session.Source); ok {
				meta := a.currentViewMeta
				meta.TurnCount = len(m.Session.Turns)
				meta.TokenCount = m.Session.TokenCount
				view := &SessionView{
					Meta:            meta,
					Session:         m.Session,
					Topics:          m.Topics,
					TurnsLoadedFrom: 0,
					TurnsLoadedTo:   len(m.Session.Turns),
					TotalTurns:      len(m.Session.Turns),
				}
				view.Live = a.liveIndex.Lookup(view.Meta.ID)
				a.currentView = view
				if view.Live != nil {
					a.landing = NewLandingModel(view, a.theme)
					if a.width > 0 {
						lm, _ := a.landing.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
						a.landing = lm.(LandingModel)
					}
					a.state = stateLanding
					return a, nil
				}
			}
		}
		a.state = stateOverview
		return a, nil
	case SessionLoadErrMsg:
		if live := a.liveIndex.Lookup(a.loading); live != nil {
			view := &SessionView{
				Meta:    a.currentViewMeta,
				Session: &session.Session{Source: a.currentViewMeta.Kind},
				Live:    live,
			}
			a.currentView = view
			a.landing = NewLandingModel(view, a.theme)
			if a.width > 0 {
				lm, _ := a.landing.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
				a.landing = lm.(LandingModel)
			}
			a.state = stateLanding
			return a, nil
		}
		a.prevState = a.state
		a.state = stateError
		a.lastErr = fmt.Sprintf("load %s: %v", m.Path, m.Err)
		return a, nil
	case ReturnToPickerMsg:
		a.state = stateList
		return a, nil
	case OpenOverviewMsg:
		a.state = stateOverview
		return a, nil
	case OpenResumeOverlayMsg:
		if a.currentView != nil {
			a.resumeOverlay = NewResumeOverlay(a.currentView.Meta.Kind, a.currentView.Meta.ID, a.theme)
			if a.width > 0 {
				rm, _ := a.resumeOverlay.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
				a.resumeOverlay = rm.(ResumeOverlayModel)
			}
			a.overlay = ovResume
		}
		return a, nil
	case CloseResumeOverlayMsg:
		a.overlay = ovNone
		return a, nil
	case OpenReplayMsg:
		a.replay = NewReplay(a.session, a.topicsCache, a.theme)
		a.replay = a.replay.SetSize(a.width, a.height).(Replay)
		a.state = stateReplay
		return a, a.replay.Init()
	case ReturnToOverviewMsg:
		a.state = stateOverview
		return a, nil
	case RestoreRequestedMsg:
		a.restorePath = m.ID + ":" + string(m.Kind)
		a.restoreID = m.ID
		a.restoreKind = m.Kind
		a.restoreModal = NewConfirm("Restore from backup?", "This will overwrite the current session file with the backup.", a.theme)
		a.state = stateConfirmRestore
		return a, nil
	case RestoreDoneMsg:
		a.overview = NewOverview(a.session, a.topicsCache, a.theme, a.cfg.GapThresholdSeconds, a.registry)
		a.state = stateList
		var store backend.SessionStore
		if a.registry != nil {
			store, _ = a.registry.Store(session.SourceClaude)
		}
		return a, rescanCmd(store)
	case RestoreErrMsg:
		a.lastErr = m.Err.Error()
		a.prevState = a.state
		a.state = stateError
		return a, nil
	case RescanDoneMsg:
		if m.Metas != nil {
			a.picker = a.picker.ReplaceMetas(m.Metas)
		}
		return a, nil
	case PruneReloadMsg:
		if m.Session != nil {
			a.session = m.Session
			a.topicsCache = m.Topics
			a.overview = NewOverview(m.Session, m.Topics, a.theme, a.cfg.GapThresholdSeconds, a.registry)
			if a.width > 0 {
				om, _ := a.overview.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
				a.overview = om.(Overview)
			}
		}
		return a, rescanAllStoresCmd(a.registry)
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
	case stateLanding:
		lm, cmd := a.landing.Update(msg)
		a.landing = lm.(LandingModel)
		return a, cmd
	case stateOverview:
		om, cmd := a.overview.Update(msg)
		a.overview = om.(Overview)
		return a, cmd
	case stateReplay:
		rm, cmd := a.replay.Update(msg)
		a.replay = rm.(Replay)
		return a, cmd
	case stateConfirmRestore:
		if km, ok := msg.(tea.KeyMsg); ok {
			m, _ := a.restoreModal.Update(km)
			c := m.(Confirm)
			a.restoreModal = c
			if c.Done() {
				if c.Confirmed() {
					return a, restoreViaRegistryCmd(a.restoreID, a.restoreKind, a.registry)
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
	case stateLanding:
		base = a.landing.View()
	case stateOverview:
		base = a.overview.View()
	case stateReplay:
		base = a.replay.View()
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
	case ovResume:
		return a.resumeOverlay.View()
	}

	// On non-picker screens, show a live-count badge in the view.
	// TODO: append the badge to the actual footer line; for now just return base.
	_ = a.liveIndex
	return base
}

type RestoreDoneMsg struct{ Path string }
type RestoreErrMsg struct{ Err error }
type RescanDoneMsg struct {
	Metas []backend.SessionMeta
}

func restoreViaRegistryCmd(id string, kind session.SourceKind, reg *backend.Registry) tea.Cmd {
	return func() tea.Msg {
		if reg == nil {
			return RestoreErrMsg{Err: editor.ErrNoBackup}
		}
		ed, ok := reg.Editor(kind)
		if !ok {
			return RestoreErrMsg{Err: editor.ErrNoBackup}
		}
		if err := ed.RestoreBackup(context.Background(), id); err != nil {
			return RestoreErrMsg{Err: err}
		}
		return RestoreDoneMsg{Path: id}
	}
}

// handleSlowTick runs all detectors, reconciles the live index, and manages
// the fast ticker and failure banner.
func (a App) handleSlowTick(_ interface{}) (App, tea.Cmd) {
	if a.LiveDisabled || a.registry == nil {
		return a, nil
	}
	// Skip while overlay is active.
	if a.overlayActive() {
		return a, slowTickCmd()
	}

	snap, err := a.scanner.Scan(a.ctx)
	if err != nil {
		a.scanFailCount++
		a.lastScanErr = err
		if a.scanFailCount >= 3 && !a.cfg.LiveDetectionLastOK.IsZero() {
			a.picker.banner = "live detection paused · press ? for details"
		}
		return a, slowTickCmd()
	}
	a.scanFailCount = 0
	a.lastScanErr = nil
	a.picker.banner = ""

	// Run all detectors.
	var detected []*backend.LiveSession
	for _, d := range a.registry.Detectors() {
		lives, err := d.DetectLive(a.ctx, snap)
		if err != nil {
			slog.Warn("detector failed", "kind", d.Kind(), "err", err)
			continue
		}
		for i := range lives {
			cp := lives[i]
			detected = append(detected, &cp)
		}
	}

	// Reconcile with hysteresis.
	_ = a.liveIndex.Reconcile(detected)

	// Push the reconciled snapshot into the picker so its rows render the
	// live pulse / status / current-task. Without this the picker stays
	// visually ended-only even when DetectLive returned matches.
	a.picker.SetLiveIndex(a.liveIndex.SnapshotMap())

	// Start or stop the fast ticker.
	var cmds []tea.Cmd
	liveCount := len(a.liveIndex.Snapshot())
	if liveCount > 0 && !a.fastActive {
		a.fastActive = true
		cmds = append(cmds, fastTickCmd())
	} else if liveCount == 0 {
		a.fastActive = false
	}
	cmds = append(cmds, slowTickCmd())
	return a, tea.Batch(cmds...)
}

// handleFastTick performs incremental loads for the currently-open live
// session. We scope the tail to the view the user is actively looking at:
// other live sessions have their status refreshed by the slow tick detector
// pass, and tailing them too would multiply DB work with no UI benefit.
func (a App) handleFastTick() (App, tea.Cmd) {
	if a.LiveDisabled {
		return a, nil
	}
	if a.overlayActive() {
		return a, fastTickCmd()
	}

	liveCount := len(a.liveIndex.Snapshot())
	if !shouldRunFastTick(liveCount, false) {
		a.fastActive = false
		return a, nil
	}

	// Only tail if the user is actively viewing a live session.
	if a.currentView != nil && a.currentView.Live != nil && a.registry != nil {
		store, ok := a.registry.Store(a.currentView.Meta.Kind)
		if ok {
			turns, newCur, err := store.LoadIncremental(a.ctx, a.currentView.Meta.ID, a.currentView.Cursor)
			switch {
			case err != nil:
				slog.Warn("fast-tick incremental load failed",
					"session", a.currentView.Meta.ID,
					"kind", a.currentView.Meta.Kind,
					"err", err)
			case len(turns) > 0:
				a.currentView.Append(turns, newCur)
				slog.Debug("fast-tick appended turns",
					"session", a.currentView.Meta.ID, "count", len(turns))
			default:
				// No new turns — still advance the cursor if it changed
				// (e.g., cold-cursor fall-through in OC).
				a.currentView.Cursor = newCur
			}
		}
	}

	return a, fastTickCmd()
}

func rescanCmd(store backend.SessionStore) tea.Cmd {
	if store == nil {
		return nil
	}
	return func() tea.Msg {
		metas, _ := store.Scan(context.Background())
		return RescanDoneMsg{Metas: metas}
	}
}

func rescanAllStoresCmd(reg *backend.Registry) tea.Cmd {
	if reg == nil {
		return nil
	}
	return func() tea.Msg {
		var metas []backend.SessionMeta
		for _, s := range reg.Stores() {
			ms, _ := s.Scan(context.Background())
			metas = append(metas, ms...)
		}
		return RescanDoneMsg{Metas: metas}
	}
}
