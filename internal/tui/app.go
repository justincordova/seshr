package tui

import (
	"bytes"
	"context"
	"errors"
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
	fastGen       int  // fast-tick chain generation; stale ticks/results are dropped
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
	// Only input messages are gated. Non-input messages (tick chains, async
	// load results, spinner frames) must still be processed — dropping a
	// liveSlowMsg/liveFastMsg here would permanently kill its
	// self-perpetuating tick chain, and dropping a SessionLoadedMsg would
	// strand the app in stateLoading.
	if a.overlayActive() {
		switch km := msg.(type) {
		case tea.KeyMsg:
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
			return a, nil
		case tea.MouseMsg:
			// Don't let mouse input leak through to the base screen.
			return a, nil
		}
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
	if fm, ok := msg.(liveFastMsg); ok {
		return a.handleFastTick(fm.Gen)
	}

	// ── Async tick results: apply state and reschedule the chains ────────────
	if dm, ok := msg.(liveDetectDoneMsg); ok {
		return a.handleDetectDone(dm)
	}
	if fm, ok := msg.(fastLoadDoneMsg); ok {
		return a.handleFastLoadDone(fm)
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
			// Clear state from any previously loaded session so the landing
			// page's `t`/`r` actions don't open a stale (or nil) session.
			a.session = nil
			a.topicsCache = nil
			a.overview = NewOverview(nil, nil, a.theme, a.cfg.GapThresholdSeconds, a.registry)
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
		// The tail is scoped to the view the user is actively looking at;
		// closing it must stop the per-tick incremental loads.
		a.currentView = nil
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
		if a.session == nil {
			// Nothing loaded (e.g. landing page shown after a load error).
			return a, nil
		}
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
		// Rescan every store: a Claude-only rescan would replace the picker's
		// meta list and wipe all OpenCode sessions from it.
		return a, rescanAllStoresCmd(a.registry)
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
			// Reset the live view too: its turns and cursor still describe
			// the pre-prune state. For OpenCode the cursor stays "valid"
			// after a prune (it's a (time,id) tuple), so without this the
			// next fast tick would append onto the stale turn list and
			// syncLiveSessionState would re-publish the pruned turns.
			if a.currentView != nil && a.currentView.Meta.ID == m.Session.ID {
				a.currentView.Reset(m.Session, m.Cursor)
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

// liveDetectDoneMsg carries the result of an async scan + detect pass.
type liveDetectDoneMsg struct {
	detected []*backend.LiveSession
	err      error
}

// fastLoadDoneMsg carries the result of an async incremental load (or the
// full reload performed after cursor invalidation).
type fastLoadDoneMsg struct {
	gen        int // fast-tick chain generation this load belongs to
	sessionID  string
	kind       session.SourceKind
	fromCursor backend.Cursor
	turns      []session.Turn
	newCur     backend.Cursor
	rebuilt    *session.Session // non-nil when the cursor was invalidated and a clean Load ran
	err        error
}

// Timeouts for the async tick commands. The tick chains are suspended while
// a command is in flight (that's how overlap is prevented), so a command
// that never returns — lsof wedged on a dead mount, a stuck DB — would
// otherwise silently kill live detection or tailing forever. A timeout
// surfaces as an error result, which reschedules the chain.
const (
	detectTimeout   = 8 * time.Second
	fastLoadTimeout = 15 * time.Second
)

// handleSlowTick kicks off the async scan+detect pass. Process scanning
// (ps + per-PID lsof) and detector DB queries must never run on the UI
// thread — a slow disk or wedged subprocess would freeze rendering and
// input. The tick chain is rescheduled when the result message is applied,
// so scans cannot overlap.
func (a App) handleSlowTick(_ interface{}) (App, tea.Cmd) {
	if a.LiveDisabled || a.registry == nil {
		return a, nil
	}
	// Skip the work while an overlay is open, but keep the chain alive.
	if a.overlayActive() {
		return a, slowTickCmd()
	}
	return a, detectLiveCmd(a.ctx, a.scanner, a.registry)
}

func detectLiveCmd(ctx context.Context, scanner *backend.ProcessScanner, reg *backend.Registry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, detectTimeout)
		defer cancel()
		snap, err := scanner.Scan(ctx)
		if err != nil {
			return liveDetectDoneMsg{err: err}
		}
		var detected []*backend.LiveSession
		for _, d := range reg.Detectors() {
			lives, derr := d.DetectLive(ctx, snap)
			if derr != nil {
				slog.Warn("detector failed", "kind", d.Kind(), "err", derr)
				continue
			}
			for i := range lives {
				cp := lives[i]
				detected = append(detected, &cp)
			}
		}
		return liveDetectDoneMsg{detected: detected}
	}
}

// handleDetectDone applies the scan results: reconciles the live index,
// updates the picker, and manages the fast ticker and failure banner.
func (a App) handleDetectDone(m liveDetectDoneMsg) (App, tea.Cmd) {
	if m.err != nil {
		a.scanFailCount++
		a.lastScanErr = m.err
		if a.scanFailCount >= 3 && !a.cfg.LiveDetectionLastOK.IsZero() {
			a.picker.banner = "live detection paused · press ? for details"
		}
		return a, slowTickCmd()
	}
	a.scanFailCount = 0
	a.lastScanErr = nil
	a.picker.banner = ""
	// Record the success so the failure banner (which only shows when live
	// detection *used to* work) can ever trigger.
	a.cfg.LiveDetectionLastOK = time.Now()

	// Reconcile with hysteresis.
	_ = a.liveIndex.Reconcile(m.detected)

	// Push the reconciled snapshot into the picker so its rows render the
	// live pulse / status / current-task. Without this the picker stays
	// visually ended-only even when DetectLive returned matches.
	a.picker.SetLiveIndex(a.liveIndex.SnapshotMap())

	// Start or stop the fast ticker. Starting bumps the generation so any
	// remnant of an older chain (e.g. a load still in flight from before
	// the chain was stopped) is dropped instead of resuming alongside the
	// new one — without the gen, a slow in-flight load + a stop/start cycle
	// would leave two chains ticking forever.
	var cmds []tea.Cmd
	liveCount := len(a.liveIndex.Snapshot())
	if liveCount > 0 && !a.fastActive {
		a.fastActive = true
		a.fastGen++
		cmds = append(cmds, fastTickCmd(a.fastGen))
	} else if liveCount == 0 {
		a.fastActive = false
		a.fastGen++
	}
	cmds = append(cmds, slowTickCmd())
	return a, tea.Batch(cmds...)
}

// handleFastTick kicks off the async incremental load for the currently-open
// live session. We scope the tail to the view the user is actively looking
// at: other live sessions have their status refreshed by the slow tick
// detector pass, and tailing them too would multiply DB work with no UI
// benefit. The chain is rescheduled when the result message is applied, so
// loads cannot overlap.
func (a App) handleFastTick(gen int) (App, tea.Cmd) {
	// Tick from a superseded chain: drop it without rescheduling.
	if gen != a.fastGen {
		return a, nil
	}
	if a.LiveDisabled {
		return a, nil
	}
	if a.overlayActive() {
		return a, fastTickCmd(a.fastGen)
	}

	liveCount := len(a.liveIndex.Snapshot())
	if !shouldRunFastTick(liveCount, false) {
		a.fastActive = false
		a.fastGen++
		return a, nil
	}

	// Only tail if the user is actively viewing a live session.
	if a.currentView != nil && a.currentView.Live != nil && a.registry != nil {
		if store, ok := a.registry.Store(a.currentView.Meta.Kind); ok {
			return a, incrementalLoadCmd(a.ctx, store, a.currentView.Meta.ID, a.currentView.Cursor, a.fastGen)
		}
	}

	return a, fastTickCmd(a.fastGen)
}

func incrementalLoadCmd(ctx context.Context, store backend.SessionStore, id string, cur backend.Cursor, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, fastLoadTimeout)
		defer cancel()
		turns, newCur, err := store.LoadIncremental(ctx, id, cur)
		if errors.Is(err, backend.ErrCursorInvalid) {
			// File rotated/truncated/pruned under us: appending would
			// duplicate every turn. Rebuild from a clean load.
			sess, freshCur, lerr := store.Load(ctx, id)
			if lerr != nil {
				return fastLoadDoneMsg{gen: gen, sessionID: id, kind: store.Kind(), fromCursor: cur,
					err: fmt.Errorf("rebuild after cursor invalidation: %w", lerr)}
			}
			return fastLoadDoneMsg{gen: gen, sessionID: id, kind: store.Kind(), fromCursor: cur,
				rebuilt: sess, newCur: freshCur}
		}
		return fastLoadDoneMsg{gen: gen, sessionID: id, kind: store.Kind(), fromCursor: cur,
			turns: turns, newCur: newCur, err: err}
	}
}

// handleFastLoadDone applies the incremental-load result to the current view.
func (a App) handleFastLoadDone(m fastLoadDoneMsg) (App, tea.Cmd) {
	// Result from a superseded chain: drop it without rescheduling.
	if m.gen != a.fastGen {
		return a, nil
	}
	// Discard results computed for a view that's gone or has moved on
	// (closed, re-opened, pruned+reloaded). The cursor comparison ensures we
	// only apply a delta against the exact state it was computed from.
	if a.currentView == nil || a.currentView.Meta.ID != m.sessionID ||
		!cursorsEqual(a.currentView.Cursor, m.fromCursor) {
		return a, fastTickCmd(a.fastGen)
	}
	switch {
	case m.err != nil:
		slog.Warn("fast-tick incremental load failed",
			"session", m.sessionID, "kind", m.kind, "err", m.err)
	case m.rebuilt != nil:
		a.currentView.Reset(m.rebuilt, m.newCur)
		a.syncLiveSessionState()
		slog.Info("fast-tick view rebuilt after cursor invalidation",
			"session", m.sessionID, "turns", len(m.rebuilt.Turns))
	case len(m.turns) > 0:
		a.currentView.Append(m.turns, m.newCur)
		a.syncLiveSessionState()
		slog.Debug("fast-tick appended turns",
			"session", m.sessionID, "count", len(m.turns))
	default:
		// No new turns — still advance the cursor if it changed
		// (e.g., cold-cursor fall-through in OC).
		a.currentView.Cursor = m.newCur
	}
	return a, fastTickCmd(a.fastGen)
}

func cursorsEqual(x, y backend.Cursor) bool {
	return x.Kind == y.Kind && bytes.Equal(x.Data, y.Data)
}

// syncLiveSessionState propagates fast-tick mutations of the current view
// (appends, evictions, full rebuilds) to the screens that hold their own
// references: a.session/a.topicsCache (used to open Replay) and the Overview
// (whose topic indices would otherwise go stale against the shifted slice).
func (a *App) syncLiveSessionState() {
	if a.currentView == nil {
		return
	}
	// Only when the view is the session the rest of the app has open.
	if a.session != nil && a.session.ID != "" && a.currentView.Session != nil &&
		a.currentView.Session.ID != "" && a.session.ID != a.currentView.Session.ID {
		return
	}
	if a.session != nil {
		a.session = a.currentView.Session
		a.topicsCache = a.currentView.Topics
	}
	a.overview.SyncLiveTurns(a.currentView.Session, a.currentView.Topics, a.currentView.TurnsLoadedFrom)
}

func rescanAllStoresCmd(reg *backend.Registry) tea.Cmd {
	if reg == nil {
		return nil
	}
	return func() tea.Msg {
		var metas []backend.SessionMeta
		for _, s := range reg.Stores() {
			ms, err := s.Scan(context.Background())
			if err != nil {
				slog.Warn("rescan store failed", "kind", s.Kind(), "err", err)
				continue
			}
			metas = append(metas, ms...)
		}
		return RescanDoneMsg{Metas: metas}
	}
}
