package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
)

const (
	sidebarMinWidth  = 16
	sidebarMaxWidth  = 24
	narrowBreakpoint = 80
)

// Replay is the Bubbletea model for the Replay screen (SPEC §3.3).
type Replay struct {
	sess           *parser.Session
	topicsList     []topics.Topic
	cursor         int
	expandedTool   int
	wrap           bool
	showThinking   bool
	autoPlay       bool
	speed          int
	width          int
	height         int
	keys           ReplayKeys
	styles         Styles
	theme          Theme
	vp             viewport.Model
	search         SearchBar
	searchHasQuery bool
	sidebarFocus   bool
	sidebarCursor  int
}

// NewReplay constructs a Replay model with sensible defaults.
func NewReplay(sess *parser.Session, ts []topics.Topic) Replay {
	th := CatppuccinMocha()
	return Replay{
		sess:         sess,
		topicsList:   ts,
		cursor:       0,
		expandedTool: -1,
		wrap:         true,
		showThinking: false,
		speed:        5,
		keys:         DefaultReplayKeys(),
		styles:       NewStyles(th),
		theme:        th,
		vp:           viewport.New(80, 20),
		search:       NewSearchBar(),
	}
}

func (m Replay) Init() tea.Cmd         { return nil }
func (m Replay) Cursor() int           { return m.cursor }
func (m Replay) WrapEnabled() bool     { return m.wrap }
func (m Replay) ThinkingVisible() bool { return m.showThinking }
func (m Replay) AutoPlaying() bool     { return m.autoPlay }
func (m Replay) Speed() int            { return m.speed }
func (m Replay) ToolExpanded() bool    { return m.expandedTool >= 0 }

func (m Replay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				if idx, ok := m.search.CurrentMatch(); ok {
					m.cursor = idx
				}
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
			case "n":
				if idx, ok := m.search.NextMatch(); ok {
					m.cursor = idx
				}
				return m, nil
			case "N":
				if idx, ok := m.search.PrevMatch(); ok {
					m.cursor = idx
				}
				return m, nil
			}
		}

		// digit speed keys 1-9
		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			m.speed = int(msg.Runes[0] - '0')
			if m.autoPlay {
				return m, AutoPlayCmd(SpeedToDelay(m.speed))
			}
			return m, nil
		}
		switch {
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
						m.cursor = t.TurnIndices[0]
					}
				}
				m.sidebarFocus = false
				return m, nil
			case "esc":
				m.sidebarFocus = false
				return m, nil
			}
			return m, nil
		case key.Matches(msg, m.keys.Next):
			if m.cursor < len(m.sess.Turns)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Prev):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.NextTopic):
			m.cursor = m.nextTopicStart()
		case key.Matches(msg, m.keys.PrevTopic):
			m.cursor = m.prevTopicStart()
		case key.Matches(msg, m.keys.ToggleThinking):
			m.showThinking = !m.showThinking
		case key.Matches(msg, m.keys.ToggleWrap):
			m.wrap = !m.wrap
		case key.Matches(msg, m.keys.AutoPlay):
			m.autoPlay = !m.autoPlay
			if m.autoPlay {
				return m, AutoPlayCmd(SpeedToDelay(m.speed))
			}
		case key.Matches(msg, m.keys.Expand):
			if m.cursor >= 0 && m.cursor < len(m.sess.Turns) {
				curTurn := m.sess.Turns[m.cursor]
				if len(curTurn.ToolResults) > 0 {
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
			return m, func() tea.Msg { return ReturnToOverviewMsg{} }
		default:
			if msg.String() == "q" {
				return m, func() tea.Msg { return ReturnToOverviewMsg{} }
			}
		}
		return m, nil

	case TickMsg:
		if !m.autoPlay {
			return m, nil
		}
		if m.cursor >= len(m.sess.Turns)-1 {
			m.autoPlay = false
			return m, nil
		}
		m.cursor++
		return m, AutoPlayCmd(SpeedToDelay(m.speed))

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 1
	}
	return m, nil
}

// SetSize updates layout dimensions.
func (m Replay) SetSize(w, h int) tea.Model {
	m.width = w
	m.height = h
	m.vp.Width = w
	m.vp.Height = h - 1
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
	if cur < 0 || cur >= len(m.topicsList)-1 {
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
	if cur < 0 {
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

	progress := ""
	if m.sess != nil && len(m.sess.Turns) > 0 {
		progress = dimStyle.Render(fmt.Sprintf("Turn %d/%d", m.cursor+1, len(m.sess.Turns)))
	}
	right := progress + "  " + dimStyle.Render("esc ") + keyStyle.Render("back")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1).
		Background(colMantle).
		Render(row)
}

func (m Replay) renderFooter() string {
	hints := []string{
		kbd("←→/hl", "turns"),
		kbd("space", "auto"),
		kbd("1-9", "speed"),
		kbd("[/]", "topics"),
	}
	if m.width >= narrowBreakpoint {
		hints = append(hints, kbd("tab", "sidebar"))
	}
	hints = append(hints,
		kbd("t", "think"),
		kbd("w", "wrap"),
		kbd("/", "search"),
	)
	if m.searchHasQuery {
		hints = append(hints, kbd("n/N", "next/prev"))
	}
	hints = append(hints, kbd("esc/q", "back"))
	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1).
		Background(colMantle).
		Render(joinHints(hints...))
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

func (m Replay) renderMain(width int) string {
	if m.cursor < 0 || m.cursor >= len(m.sess.Turns) {
		return ""
	}
	turn := m.sess.Turns[m.cursor]
	prev := time.Time{}
	if m.cursor > 0 {
		prev = m.sess.Turns[m.cursor-1].Timestamp
	}

	var b strings.Builder
	b.WriteString(RenderTurnHeader(turn, prev, width, m.styles, m.theme))
	b.WriteString("\n")

	bodyWidth := width
	if !m.wrap {
		bodyWidth = 10_000
	}
	body, _ := RenderMarkdownBody(turn.Content, bodyWidth)
	b.WriteString(body)

	for _, tc := range turn.ToolCalls {
		b.WriteString("\n")
		b.WriteString(RenderToolCall(tc, width, m.styles))
	}
	for _, tr := range turn.ToolResults {
		b.WriteString("\n")
		b.WriteString(RenderToolResult(tr.Content, width, m.styles))
	}

	if m.showThinking && turn.Thinking != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.Thinking.Render(turn.Thinking))
	}
	return b.String()
}

func (m Replay) renderSidebarPanel(ts []topics.Topic, active, width, height int, focused bool) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	if focused {
		style = activeBoxStyle.Width(width - 2).Height(height - 2)
	}
	body := RenderSidebar(ts, active, width-4, m.theme)
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

func (m Replay) renderMainPanel(width, height int) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	body := m.renderMain(width - 4)
	return style.Render(body)
}
