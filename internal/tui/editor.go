package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/editor"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
)

type Editor struct {
	sess     *parser.Session
	topics   []topics.Topic
	cursor   int
	selected map[int]bool
	width    int
	height   int
	keys     EditorKeys
	styles   Styles
	status   string
}

func NewEditor(sess *parser.Session, ts []topics.Topic) Editor {
	return Editor{
		sess:     sess,
		topics:   ts,
		selected: map[int]bool{},
		keys:     DefaultEditorKeys(),
		styles:   NewStyles(CatppuccinMocha()),
	}
}

func (m Editor) Init() tea.Cmd         { return nil }
func (m Editor) Cursor() int           { return m.cursor }
func (m Editor) IsSelected(i int) bool { return m.selected[i] }

func (m Editor) SetSize(w, h int) tea.Model {
	m.width = w
	m.height = h
	return m
}

func (m Editor) selectedCount() int {
	n := 0
	for _, v := range m.selected {
		if v {
			n++
		}
	}
	return n
}

func (m Editor) tokensFreed() int {
	sum := 0
	for i, sel := range m.selected {
		if sel && i >= 0 && i < len(m.topics) {
			sum += m.topics[i].TokenCount
		}
	}
	return sum
}

func (m Editor) currentSelection() editor.Selection {
	sel := editor.Selection{Topics: map[int]bool{}}
	for i, v := range m.selected {
		if v {
			sel.Topics[i] = true
		}
	}
	return editor.ExpandSelection(m.sess, m.topics, sel)
}

func (m Editor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case PruneDoneMsg:
		m.status = fmt.Sprintf("pruned %d turns — press esc to return", msg.RemovedTurns)
		m.selected = map[int]bool{}
		return m, LoadSessionCmd(m.sess.Path)
	case PruneErrMsg:
		m.status = msg.Err.Error()
		return m, nil
	case SessionLoadedMsg:
		m.sess = msg.Session
		m.topics = msg.Topics
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m Editor) handleKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(km, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(km, m.keys.Down):
		if m.cursor < len(m.topics)-1 {
			m.cursor++
		}
	case key.Matches(km, m.keys.Toggle):
		m.selected[m.cursor] = !m.selected[m.cursor]
	case key.Matches(km, m.keys.SelectAll):
		for i := range m.topics {
			m.selected[i] = true
		}
	case key.Matches(km, m.keys.SelectNone):
		m.selected = map[int]bool{}
	case key.Matches(km, m.keys.Prune):
		if m.selectedCount() == 0 {
			return m, nil
		}
		sel := m.currentSelection()
		m.status = "pruning…"
		return m, pruneCmd(m.sess, sel)
	case key.Matches(km, m.keys.Cancel):
		return m, func() tea.Msg { return ReturnToOverviewMsg{} }
	}
	return m, nil
}

func (m Editor) View() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("EDIT MODE — select topics to prune"))
	b.WriteString("\n\n")
	for i, t := range m.topics {
		b.WriteString(RenderCheckboxRow(i, t, m.selected[i], i == m.cursor, m.width, m.styles))
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(RenderSelectionFooter(m.selectedCount(), 0, m.tokensFreed()))
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.Error.Render(m.status))
	}
	b.WriteString("\n")
	b.WriteString(m.styles.Hint.Render("space Select  a All  A None  p Prune  esc Cancel"))
	return b.String()
}

func pruneCmd(sess *parser.Session, sel editor.Selection) tea.Cmd {
	return func() tea.Msg {
		if err := editor.PruneSession(sess, sel); err != nil {
			return PruneErrMsg{Err: err}
		}
		return PruneDoneMsg{RemovedTurns: len(sel.Turns)}
	}
}

type PruneDoneMsg struct{ RemovedTurns int }
type PruneErrMsg struct{ Err error }
