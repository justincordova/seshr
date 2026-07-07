package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
)

const (
	sidebarMinWidth  = 16
	sidebarMaxWidth  = 24
	narrowBreakpoint = 80
)

// Replay is the Bubbletea model for the Replay screen (SPEC §3.3).
type Replay struct {
	sess               *session.Session
	topicsList         []topics.Topic
	cursor             int
	expandedTool       int
	slim               bool
	showThinking       bool
	autoPlay           bool
	playGen            int
	speed              int
	width              int
	height             int
	keys               ReplayKeys
	styles             Styles
	theme              Theme
	vp                 viewport.Model
	mainVP             viewport.Model
	search             SearchBar
	searchHasQuery     bool
	searchResultCursor int
	searchScrollTop    int
	sidebarFocus       bool
	sidebarCursor      int
	mainPrimed         bool // mainVP has had content set at least once
}

// NewReplay constructs a Replay model with sensible defaults.
func NewReplay(sess *session.Session, ts []topics.Topic, th Theme) Replay {
	return Replay{
		sess:         sess,
		topicsList:   ts,
		cursor:       0,
		expandedTool: -1,
		showThinking: false,
		speed:        5,
		keys:         DefaultReplayKeys(),
		styles:       NewStyles(th),
		theme:        th,
		vp:           viewport.New(80, 20),
		mainVP:       viewport.New(80, 20),
		search:       NewSearchBar(),
	}
}

func (m Replay) Init() tea.Cmd         { return nil }
func (m Replay) Cursor() int           { return m.cursor }
func (m Replay) ThinkingVisible() bool { return m.showThinking }
func (m Replay) AutoPlaying() bool     { return m.autoPlay }
func (m Replay) PlayGen() int          { return m.playGen }
func (m Replay) Speed() int            { return m.speed }
func (m Replay) ToolExpanded() bool    { return m.expandedTool >= 0 }

// MainScrollOffset exposes the main panel's scroll position for tests.
func (m Replay) MainScrollOffset() int { return m.mainVP.YOffset }

// Update wraps update with main-viewport content syncing. View operates on a
// COPY of the model, so a SetContent there can never persist — scrolling
// would forever operate on an empty viewport. Whenever something that
// affects the rendered turn changes (cursor, slim, thinking, dimensions),
// the persistent mainVP's content is re-rendered here, in Update, where the
// mutation sticks.
func (m Replay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.contentSig()
	res, cmd := m.update(msg)
	next, ok := res.(Replay)
	if !ok {
		return res, cmd
	}
	if !next.mainPrimed || next.contentSig() != before {
		next.syncMainContent()
	}
	return next, cmd
}

// contentSig captures every model field the main panel's rendered body
// depends on. A change in the signature invalidates mainVP's content.
type replayContentSig struct {
	cursor, width, height int
	slim, thinking        bool
}

func (m Replay) contentSig() replayContentSig {
	return replayContentSig{
		cursor:   m.cursor,
		width:    m.width,
		height:   m.height,
		slim:     m.slim,
		thinking: m.showThinking,
	}
}

// syncMainContent renders the current turn into the persistent main viewport.
func (m *Replay) syncMainContent() {
	m.mainVP.SetContent(m.renderMain(m.mainVP.Width))
	m.mainPrimed = true
}

func (m Replay) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.search.Active() {
			switch msg.String() {
			case "esc":
				m.search.Close()
				m.searchHasQuery = false
				return m, nil
			case "enter":
				m.search.Commit()
				m.applyTurnSearch()
				m.searchResultCursor = 0
				m.searchScrollTop = 0
				m.searchHasQuery = m.search.Query() != ""
				return m, nil
			default:
				m.search.Update(msg)
				m.applyTurnSearch()
				return m, nil
			}
		}

		// n/N for search next/prev when a committed query exists
		if m.searchHasQuery && !m.search.Active() {
			switch msg.String() {
			case "up", "k":
				if m.searchResultCursor > 0 {
					m.searchResultCursor--
					m.recalcSearchScroll()
				}
				return m, nil
			case "down", "j":
				if m.searchResultCursor < m.search.MatchCount()-1 {
					m.searchResultCursor++
					m.recalcSearchScroll()
				}
				return m, nil
			case "enter":
				matches := m.search.Matches()
				if m.searchResultCursor >= 0 && m.searchResultCursor < len(matches) {
					m.cursor = matches[m.searchResultCursor].Index
					m.mainVP.GotoTop()
				}
				m.searchHasQuery = false
				m.search.Close()
				return m, nil
			case "esc":
				m.searchHasQuery = false
				m.search.Close()
				return m, nil
			}
		}

		switch {
		case !m.sidebarFocus && key.Matches(msg, m.keys.Top):
			m.cursor = 0
			if m.slim {
				m.skipInvisibleForward()
			}
			m.mainVP.GotoTop()
			return m, nil
		case !m.sidebarFocus && key.Matches(msg, m.keys.Bottom):
			if n := len(m.sess.Turns); n > 0 {
				m.cursor = n - 1
				if m.slim {
					m.skipInvisibleBackward()
				}
				// Content must be set BEFORE GotoBottom so the max offset is
				// computed against the new turn's body, not stale content.
				m.syncMainContent()
				m.mainVP.GotoBottom()
			}
			return m, nil
		case !m.sidebarFocus && key.Matches(msg, m.keys.PageDown):
			if m.expandedTool >= 0 {
				m.vp.HalfPageDown()
			} else {
				m.mainVP.HalfPageDown()
			}
			return m, nil
		case !m.sidebarFocus && key.Matches(msg, m.keys.PageUp):
			if m.expandedTool >= 0 {
				m.vp.HalfPageUp()
			} else {
				m.mainVP.HalfPageUp()
			}
			return m, nil
		case key.Matches(msg, m.keys.SidebarFocus):
			if m.width >= narrowBreakpoint && len(m.topicsList) > 0 {
				if !m.sidebarFocus {
					m.sidebarFocus = true
					m.sidebarCursor = m.currentTopicIndex()
					if m.sidebarCursor < 0 {
						m.sidebarCursor = 0
					}
				} else {
					m.sidebarFocus = false
				}
			}
			return m, nil
		case m.sidebarFocus:
			switch msg.String() {
			case "up", "k":
				if m.sidebarCursor > 0 {
					m.sidebarCursor--
				}
				return m, nil
			case "down", "j":
				if m.sidebarCursor < len(m.topicsList)-1 {
					m.sidebarCursor++
				}
				return m, nil
			case "enter":
				if m.sidebarCursor >= 0 && m.sidebarCursor < len(m.topicsList) {
					t := m.topicsList[m.sidebarCursor]
					if len(t.TurnIndices) > 0 {
						m.jumpToTurn(t.TurnIndices[0])
					}
				}
				m.sidebarFocus = false
				return m, nil
			case "esc":
				m.sidebarFocus = false
				return m, nil
			}
			return m, nil
		default:
			if m.expandedTool >= 0 {
				switch msg.String() {
				case "up", "k":
					m.vp.ScrollUp(1)
					return m, nil
				case "down", "j":
					m.vp.ScrollDown(1)
					return m, nil
				}
			} else {
				switch msg.String() {
				case "up", "k":
					m.mainVP.ScrollUp(1)
					return m, nil
				case "down", "j":
					m.mainVP.ScrollDown(1)
					return m, nil
				}
			}
		}
		switch {
		case key.Matches(msg, m.keys.Next):
			if m.cursor < len(m.sess.Turns)-1 {
				m.cursor++
				if m.slim {
					m.skipInvisibleForward()
				}
				m.mainVP.GotoTop()
				// Moving to a new turn collapses any expanded tool-result
				// view; otherwise the user would silently move into a new
				// turn while the screen kept rendering the previous turn's
				// expanded result.
				m.expandedTool = -1
			}
		case key.Matches(msg, m.keys.Prev):
			if m.cursor > 0 {
				m.cursor--
				if m.slim {
					m.skipInvisibleBackward()
				}
				m.mainVP.GotoTop()
				m.expandedTool = -1
			}
		case key.Matches(msg, m.keys.NextTopic):
			m.jumpToTurn(m.nextTopicStart())
			m.expandedTool = -1
		case key.Matches(msg, m.keys.PrevTopic):
			m.jumpToTurn(m.prevTopicStart())
			m.expandedTool = -1
		case key.Matches(msg, m.keys.ToggleThinking):
			m.showThinking = !m.showThinking
		case key.Matches(msg, m.keys.ToggleSlim):
			m.slim = !m.slim
		case key.Matches(msg, m.keys.SpeedUp):
			if m.autoPlay {
				m.speed++
				if m.speed > 9 {
					m.speed = 9
				}
				m.playGen++
				return m, AutoPlayCmd(SpeedToDelay(m.speed), m.playGen)
			}
		case key.Matches(msg, m.keys.SpeedDown):
			if m.autoPlay {
				m.speed--
				if m.speed < 1 {
					m.speed = 1
				}
				m.playGen++
				return m, AutoPlayCmd(SpeedToDelay(m.speed), m.playGen)
			}
		case key.Matches(msg, m.keys.AutoPlay):
			m.autoPlay = !m.autoPlay
			if m.autoPlay {
				m.playGen++
				return m, AutoPlayCmd(SpeedToDelay(m.speed), m.playGen)
			}
		case key.Matches(msg, m.keys.Expand):
			if m.cursor >= 0 && m.cursor < len(m.sess.Turns) {
				curTurn := m.sess.Turns[m.cursor]
				if curTurn.IsCompactContinuation && curTurn.Content != "" {
					m.expandedTool = 0
					m.vp.SetContent(curTurn.Content)
				} else if len(curTurn.ToolResults) > 0 {
					m.expandedTool = 0
					m.vp.SetContent(curTurn.ToolResults[0].Content)
				}
			}
		case key.Matches(msg, m.keys.Search):
			m.search.Open()
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if m.expandedTool >= 0 {
				m.expandedTool = -1
				return m, nil
			}
			return m, func() tea.Msg { return ReturnToOverviewMsg{} }
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}
		return m, nil

	case TickMsg:
		// Reject ticks from a superseded chain (pause/resume or speed change
		// re-arm a new generation; tea.Tick can't cancel the old one).
		if msg.Gen != m.playGen {
			return m, nil
		}
		if !m.autoPlay {
			return m, nil
		}
		if m.cursor >= len(m.sess.Turns)-1 {
			m.autoPlay = false
			return m, nil
		}
		m.cursor++
		if m.slim {
			m.skipInvisibleForward()
		}
		return m, AutoPlayCmd(SpeedToDelay(m.speed), m.playGen)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 1
		m.syncMainVPSize()

	case tea.MouseMsg:
		if m.expandedTool >= 0 {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.mainVP.ScrollUp(3)
		case tea.MouseButtonWheelDown:
			m.mainVP.ScrollDown(3)
		}
	}
	return m, nil
}

// syncMainVPSize
func (m *Replay) syncMainVPSize() {
	if m.width < narrowBreakpoint {
		m.mainVP.Width = m.width - 4
	} else {
		sw := m.sidebarWidth()
		mw := m.width - sw - 3
		m.mainVP.Width = mw - 4
	}
	header := m.renderHeader()
	footer := m.renderFooter()
	searchBar := m.search.View(m.width)
	fixedH := lipgloss.Height(header) + lipgloss.Height(searchBar) + lipgloss.Height(footer)
	contentH := m.height - fixedH
	if contentH < 6 {
		contentH = 6
	}
	m.mainVP.Height = contentH - 2
}

// RefreshSession swaps in the (possibly re-sliced or replaced) live session
// and re-renders the main viewport. Called by the app when the fast tick
// mutates the session this replay is open on — contentSig can't see those
// mutations (same cursor, same dimensions), and after a view rebuild the
// old pointer would be orphaned entirely.
func (m Replay) RefreshSession(sess *session.Session, ts []topics.Topic) Replay {
	m.sess = sess
	m.topicsList = ts
	if n := len(sess.Turns); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	// A committed search holds match indices into the OLD turn slice. After an
	// append/eviction/rebuild those indices point at the wrong turns (or out
	// of range), so re-run the search against the new slice and clamp the
	// result cursor. Otherwise pressing enter would jump to a stale index.
	if m.searchHasQuery {
		m.applyTurnSearch()
		if n := m.search.MatchCount(); n == 0 {
			m.searchResultCursor = 0
		} else if m.searchResultCursor >= n {
			m.searchResultCursor = n - 1
		}
	}
	m.syncMainContent()
	return m
}

// SetSize updates layout dimensions.
func (m Replay) SetSize(w, h int) tea.Model {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h - 1
	m.syncMainVPSize()
	m.syncMainContent()
	return m
}

func (m Replay) currentTopicIndex() int {
	for i, t := range m.topicsList {
		for _, idx := range t.TurnIndices {
			if idx == m.cursor {
				return i
			}
		}
	}
	return -1
}

func (m Replay) nextTopicStart() int {
	cur := m.currentTopicIndex()
	// Cursor sits on an uncategorized turn (system/summary/continuation turns
	// belong to no topic). Advance to the first topic that starts after the
	// cursor rather than jumping to the end of the session.
	if cur < 0 {
		for _, t := range m.topicsList {
			if len(t.TurnIndices) > 0 && t.TurnIndices[0] > m.cursor {
				return t.TurnIndices[0]
			}
		}
		return len(m.sess.Turns) - 1
	}
	if cur >= len(m.topicsList)-1 {
		return len(m.sess.Turns) - 1
	}
	next := m.topicsList[cur+1]
	if len(next.TurnIndices) == 0 {
		return m.cursor
	}
	return next.TurnIndices[0]
}

func (m Replay) prevTopicStart() int {
	cur := m.currentTopicIndex()
	// Cursor sits on an uncategorized turn: fall back to the last topic that
	// starts before the cursor rather than jumping to the very top.
	if cur < 0 {
		for i := len(m.topicsList) - 1; i >= 0; i-- {
			t := m.topicsList[i]
			if len(t.TurnIndices) > 0 && t.TurnIndices[0] < m.cursor {
				return t.TurnIndices[0]
			}
		}
		return 0
	}
	curTopic := m.topicsList[cur]
	if len(curTopic.TurnIndices) > 0 && m.cursor != curTopic.TurnIndices[0] {
		return curTopic.TurnIndices[0]
	}
	if cur == 0 {
		return 0
	}
	prev := m.topicsList[cur-1]
	if len(prev.TurnIndices) == 0 {
		return 0
	}
	return prev.TurnIndices[0]
}

// ReturnToOverviewMsg is emitted by Replay on esc (when no tool result is expanded).
type ReturnToOverviewMsg struct{}

func (m Replay) View() string { return m.renderView() }

func (m Replay) renderView() string {
	if m.expandedTool >= 0 {
		return m.renderExpanded()
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	searchBar := m.search.View(m.width)

	fixedH := lipgloss.Height(header) + lipgloss.Height(searchBar) + lipgloss.Height(footer)
	contentH := m.height - fixedH
	if contentH < 6 {
		contentH = 6
	}

	var content string
	if m.width < narrowBreakpoint {
		content = m.renderNarrow(contentH)
	} else {
		content = m.renderWide(contentH)
	}

	parts := []string{header, content}
	if searchBar != "" {
		parts = append(parts, searchBar)
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Replay) renderExpanded() string {
	return m.vp.View()
}

func (m Replay) renderHeader() string {
	left := lipgloss.NewStyle().
		Foreground(colMauve).
		Bold(true).
		Render("◆ Replay")

	var indicators []string
	if m.showThinking {
		indicators = append(indicators, lipgloss.NewStyle().Foreground(colLavender).Bold(true).Render("thinking"))
	}
	if m.slim {
		indicators = append(indicators, lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("slim"))
	}
	indicatorStr := strings.Join(indicators, " ")

	progress := ""
	if m.sess != nil && len(m.sess.Turns) > 0 {
		progress = dimStyle.Render(fmt.Sprintf("Turn %d/%d", m.cursor+1, len(m.sess.Turns)))
	}

	center := indicatorStr
	right := progress

	leftW := lipgloss.Width(left)
	centerW := lipgloss.Width(center)
	rightW := lipgloss.Width(right)
	gap := m.width - leftW - centerW - rightW - 4
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + center + "  " + right
	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1).
		Background(colMantle).
		Render(row)
}

func (m Replay) renderFooter() string {
	hints := []string{
		kbdPill("←→/hl", "turns"),
		kbdPill("space", "auto"),
		kbdPill("[/]", "topics"),
	}
	if m.width >= narrowBreakpoint {
		hints = append(hints, kbdPill("tab", "sidebar"))
	}
	hints = append(hints,
		kbdPill("t", "think"),
		kbdPill("c", "slim"),
		kbdPill("/", "search"),
	)
	if m.autoPlay {
		speedPill := lipgloss.NewStyle().
			Foreground(m.theme.Accent).
			Background(m.theme.Background).
			Padding(0, 1).
			Bold(true).
			Render(fmt.Sprintf("▶ %dx", m.speed))
		hints = append(hints, speedPill, kbdPill("+/-", "speed"))
	}
	if m.searchHasQuery {
		hints = append(hints, kbdPill("↑↓/jk", "browse"), kbdPill("enter", "jump"), kbdPill("esc", "clear"))
	} else {
		hints = append(hints, kbdPill("esc", "back"))
	}
	return renderCenteredFooter(hints, m.width)
}

func (m Replay) sidebarWidth() int {
	w := m.width / 5
	if w < sidebarMinWidth {
		w = sidebarMinWidth
	}
	if w > sidebarMaxWidth {
		w = sidebarMaxWidth
	}
	return w
}

func (m Replay) renderWide(contentH int) string {
	sw := m.sidebarWidth()
	mw := m.width - sw - 3
	activeTopic := m.currentTopicIndex()
	var sidebar string
	if m.sidebarFocus {
		sidebar = m.renderSidebarPanel(m.topicsList, m.sidebarCursor, sw, contentH, true)
	} else {
		sidebar = m.renderSidebarPanel(m.topicsList, activeTopic, sw, contentH, false)
	}
	main := m.renderMainPanel(mw, contentH)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", main)
}

func (m Replay) renderNarrow(contentH int) string {
	header := m.renderTopicHeaderBar()
	main := m.renderMainPanel(m.width, contentH-1)
	return lipgloss.JoinVertical(lipgloss.Left, header, main)
}

func (m Replay) renderTopicHeaderBar() string {
	active := m.currentTopicIndex()
	parts := make([]string, 0, len(m.topicsList))
	for i, t := range m.topicsList {
		label := fmt.Sprintf("%d.%s", i+1, t.Label)
		if i == active {
			parts = append(parts, pill(label, m.theme.Accent, m.theme.Background))
		} else {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, " ")
}

// turnIsPreCompact returns true if the turn at turnIdx falls entirely before
// any compact boundary (i.e. it is inactive context).
func (m Replay) turnIsPreCompact(turnIdx int) bool {
	if m.sess == nil || len(m.sess.CompactBoundaries) == 0 {
		return false
	}
	earliest := m.sess.CompactBoundaries[0].TurnIndex
	for _, cb := range m.sess.CompactBoundaries[1:] {
		if cb.TurnIndex < earliest {
			earliest = cb.TurnIndex
		}
	}
	return turnIdx < earliest
}

func (m Replay) renderMain(width int) string {
	if m.cursor < 0 || m.cursor >= len(m.sess.Turns) {
		return ""
	}
	turn := m.sess.Turns[m.cursor]
	if m.slim && !m.turnVisible(turn) {
		return ""
	}
	prev := time.Time{}
	if m.cursor > 0 {
		prev = m.sess.Turns[m.cursor-1].Timestamp
	}

	preCompact := m.turnIsPreCompact(m.cursor)

	var b strings.Builder

	// Continuation summary turns render collapsed with a special badge.
	if turn.IsCompactContinuation {
		badge := pill("continuation summary", colBase, colBlue)
		tokens := m.styles.Hint.Render(fmt.Sprintf("~%d tok", turn.Tokens))
		gap := width - lipgloss.Width(badge) - lipgloss.Width(tokens)
		if gap < 1 {
			gap = 1
		}
		b.WriteString(badge + strings.Repeat(" ", gap) + tokens)
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("(press enter to expand)"))
		return b.String()
	}

	header := RenderTurnHeader(turn, prev, width, m.styles, m.theme)
	if preCompact {
		badge := lipgloss.NewStyle().Foreground(colSurface1).Render(" pre-compact")
		header += badge
	}
	b.WriteString(header)
	b.WriteString("\n")

	if turn.Content != "" {
		body, _ := RenderMarkdownBody(turn.Content, width)
		b.WriteString(strings.TrimRight(body, "\n"))
	}

	if !m.slim {
		for _, tc := range turn.ToolCalls {
			b.WriteString("\n\n")
			if tc.Name == "Agent" {
				b.WriteString(RenderAgentToolCall(tc, width, m.theme))
			} else {
				b.WriteString(RenderToolCall(tc, width, m.styles))
			}
		}
		for _, tr := range turn.ToolResults {
			b.WriteString("\n\n")
			b.WriteString(RenderToolResult(tr.Content, tr.IsError, width, m.styles))
		}
	} else {
		for _, tc := range turn.ToolCalls {
			if tc.Name == "Agent" {
				b.WriteString("\n")
				b.WriteString(RenderAgentToolCall(tc, width, m.theme))
			}
		}
	}

	if m.showThinking && turn.Thinking != "" {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Thinking.Render(turn.Thinking))
	}
	return b.String()
}

func (m Replay) turnVisible(turn session.Turn) bool {
	if turn.Content != "" {
		return true
	}
	if m.showThinking && turn.Thinking != "" {
		return true
	}
	for _, tc := range turn.ToolCalls {
		if tc.Name == "Agent" {
			return true
		}
	}
	if !m.slim {
		return len(turn.ToolCalls) > 0 || len(turn.ToolResults) > 0
	}
	return false
}

// jumpToTurn moves the cursor to idx and normalizes viewport state the same
// way the arrow-key paths do: clamp into range, skip slim-hidden turns so the
// main panel is never left blank, and reset the scroll offset to the top of
// the new turn. Topic jumps and sidebar jumps must go through here — setting
// m.cursor alone leaves mainVP scrolled to the previous turn's offset (bubbles
// SetContent only snaps when the new content is SHORTER than the old offset),
// stranding the user mid-turn.
func (m *Replay) jumpToTurn(idx int) {
	if n := len(m.sess.Turns); n > 0 {
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
	} else {
		idx = 0
	}
	m.cursor = idx
	if m.slim {
		m.skipInvisibleForward()
	}
	m.mainVP.GotoTop()
}

func (m *Replay) skipInvisibleForward() {
	for m.cursor < len(m.sess.Turns)-1 && !m.turnVisible(m.sess.Turns[m.cursor]) {
		m.cursor++
	}
}

func (m *Replay) skipInvisibleBackward() {
	for m.cursor > 0 && !m.turnVisible(m.sess.Turns[m.cursor]) {
		m.cursor--
	}
}

func (m Replay) renderSidebarPanel(ts []topics.Topic, active, width, height int, focused bool) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	if focused {
		style = activeBoxStyle.Width(width - 2).Height(height - 2)
	}
	body := RenderSidebar(m.sess, ts, active, width-4, m.theme)
	return style.Render(body)
}

func (m *Replay) applyTurnSearch() {
	if m.sess == nil || len(m.sess.Turns) == 0 {
		return
	}
	haystack := make([]string, len(m.sess.Turns))
	for i, t := range m.sess.Turns {
		haystack[i] = t.Content
		for _, tc := range t.ToolCalls {
			haystack[i] += " " + string(tc.Input)
		}
		for _, tr := range t.ToolResults {
			haystack[i] += " " + tr.Content
		}
	}
	m.search.Filter(haystack)
}

func (m *Replay) recalcSearchScroll() {
	h := m.mainVP.Height
	if h < 1 {
		h = 1
	}
	w := m.mainVP.Width
	if w < 4 {
		w = 4
	}
	m.searchScrollTop = ComputeSearchScrollTop(
		m.sess, m.search.Matches(), m.searchResultCursor,
		w, h, m.searchScrollTop,
	)
}

func (m Replay) renderMainPanel(width, height int) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	if m.searchHasQuery {
		// Search results manage their own scrolling — no viewport needed.
		// Inner height = panel height minus top/bottom border.
		innerH := height - 2
		if innerH < 1 {
			innerH = 1
		}
		body, _ := RenderSearchResults(m.sess, m.search.Matches(), m.searchResultCursor, width-4, innerH, m.searchScrollTop, m.styles, m.theme)
		return style.Render(body)
	}
	// Content is kept in sync by Update (syncMainContent) — setting it here
	// would only mutate View's copy of the model and be lost.
	return style.Render(m.mainVP.View())
}
