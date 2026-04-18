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
	sess         *parser.Session
	topicsList   []topics.Topic
	cursor       int
	expandedTool int // -1 means none expanded
	wrap         bool
	showThinking bool
	autoPlay     bool
	speed        int
	width        int
	height       int
	keys         ReplayKeys
	styles       Styles
	theme        Theme
	vp           viewport.Model
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
		// digit speed keys 1-9
		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			m.speed = int(msg.Runes[0] - '0')
			if m.autoPlay {
				return m, AutoPlayCmd(SpeedToDelay(m.speed))
			}
			return m, nil
		}
		switch {
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
		return m.vp.View()
	}
	if m.width < narrowBreakpoint {
		return m.renderNarrow()
	}
	return m.renderWide()
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

func (m Replay) renderWide() string {
	sw := m.sidebarWidth()
	mw := m.width - sw - 1
	activeTopic := m.currentTopicIndex()
	sidebar := RenderSidebar(m.topicsList, activeTopic, sw, m.theme)
	main := m.renderMain(mw)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
}

func (m Replay) renderNarrow() string {
	header := m.renderTopicHeaderBar()
	main := m.renderMain(m.width)
	return header + "\n" + main
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
	b.WriteString("\n\n")

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
