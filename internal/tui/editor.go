package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/seshly/internal/editor"
	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/topics"
)

type Editor struct {
	sess     *parser.Session
	topics   []topics.Topic
	cursor   int
	offset   int
	expanded map[int]bool
	selected map[int]bool
	width    int
	height   int
	keys     EditorKeys
	styles   Styles
	status   string
	pruning  bool
	confirm  *Confirm
}

func NewEditor(sess *parser.Session, ts []topics.Topic) Editor {
	return Editor{
		sess:     sess,
		topics:   ts,
		expanded: map[int]bool{},
		selected: map[int]bool{},
		keys:     DefaultEditorKeys(),
		styles:   NewStyles(CatppuccinMocha()),
	}
}

func (m Editor) Init() tea.Cmd         { return nil }
func (m Editor) Cursor() int           { return m.cursor }
func (m Editor) IsSelected(i int) bool { return m.selected[i] }
func (m Editor) IsExpanded(i int) bool { return m.expanded[i] }
func (m Editor) Pruning() bool         { return m.pruning }

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
	if m.confirm != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			cm, _ := m.confirm.Update(km)
			c := cm.(Confirm)
			m.confirm = &c
			if c.Done() {
				m.confirm = nil
				if c.Confirmed() {
					sel := m.currentSelection()
					m.pruning = true
					m.status = "pruning…"
					return m, pruneCmd(m.sess, sel)
				}
			}
			return m, nil
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case PruneDoneMsg:
		m.pruning = false
		m.status = fmt.Sprintf("pruned %d turns — press esc to return", msg.RemovedTurns)
		m.selected = map[int]bool{}
		return m, tea.Batch(LoadSessionCmd(m.sess.Path), clearStatusCmd())
	case PruneErrMsg:
		m.pruning = false
		m.status = msg.Err.Error()
		return m, nil
	case SessionLoadedMsg:
		m.sess = msg.Session
		m.topics = msg.Topics
		m.cursor = 0
		return m, nil
	case clearStatusMsg:
		m.status = ""
		return m, nil
	}
	return m, nil
}

func (m Editor) handleKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(km, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.offset = clampOffset(m.cursor, m.offset, len(m.topics), m.visibleCount())
		}
	case key.Matches(km, m.keys.Down):
		if m.cursor < len(m.topics)-1 {
			m.cursor++
			m.offset = clampOffset(m.cursor, m.offset, len(m.topics), m.visibleCount())
		}
	case key.Matches(km, m.keys.Toggle):
		m.selected[m.cursor] = !m.selected[m.cursor]
	case key.Matches(km, m.keys.SelectAll):
		for i := range m.topics {
			m.selected[i] = true
		}
	case key.Matches(km, m.keys.SelectNone):
		m.selected = map[int]bool{}
	case key.Matches(km, m.keys.Expand):
		m.expanded[m.cursor] = !m.expanded[m.cursor]
	case key.Matches(km, m.keys.Prune):
		if m.pruning || m.selectedCount() == 0 {
			return m, nil
		}
		sel := m.currentSelection()
		body := fmt.Sprintf("%d topics · %d turns · ~%s tokens freed\n\nThis rewrites the file and creates a .bak backup.\nType /clear in Claude Code then resume for changes to take effect.",
			m.selectedCount(), len(sel.Turns), humanize.Comma(int64(m.tokensFreed())))
		m.confirm = &Confirm{
			title:  "Prune selected topics?",
			body:   body,
			styles: m.styles,
		}
		return m, nil
	case key.Matches(km, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(km, m.keys.Cancel):
		return m, func() tea.Msg { return ReturnToOverviewMsg{} }
	}
	return m, nil
}

func (m Editor) View() string {
	if m.confirm != nil {
		return m.confirm.View()
	}

	cw := contentWidth(m.width)

	detailH := 5
	footerH := 2
	mainH := m.height - detailH - footerH - 2
	if mainH < 6 {
		mainH = 6
	}

	body := m.renderTopicList(cw-4, mainH-4)
	main := panel(" Edit Mode — select topics to prune", body, cw, mainH)

	detail := m.renderDetailPanel(cw, detailH)
	footer := m.renderFooter(cw)

	parts := []string{main, detail, footer}
	return centerBlock(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width)
}

func (m Editor) renderTopicList(width, maxLines int) string {
	var b strings.Builder

	end := m.offset + m.visibleCount()
	if end > len(m.topics) {
		end = len(m.topics)
	}
	linesUsed := 0
	for i := m.offset; i < end; i++ {
		if linesUsed > 0 {
			b.WriteByte('\n')
			linesUsed++
		}
		t := m.topics[i]
		b.WriteString(RenderCheckboxRow(i, t, m.selected[i], i == m.cursor, width, m.styles))
		linesUsed++

		if m.expanded[i] {
			for _, turnIdx := range t.TurnIndices {
				if linesUsed >= maxLines {
					break
				}
				if turnIdx < 0 || turnIdx >= len(m.sess.Turns) {
					continue
				}
				tn := m.sess.Turns[turnIdx]
				b.WriteByte('\n')
				badge := roleBadge(tn.Role)
				preview := truncate(firstLine(tn.Content), 50)
				line := fmt.Sprintf("       %s  %s  ~%d", badge, preview, tn.Tokens)
				b.WriteString(m.styles.Hint.Render(line))
				linesUsed++
			}
		}

		if linesUsed >= maxLines {
			break
		}
	}
	return b.String()
}

func (m Editor) renderDetailPanel(width, height int) string {
	sel := m.currentSelection()
	title := dimStyle.Render(
		fmt.Sprintf("%d topics · %d turns · ~%s tokens freed",
			m.selectedCount(), len(sel.Turns), humanize.Comma(int64(m.tokensFreed()))),
	)
	body := "\n" + title + "\n"
	if m.status != "" {
		if m.pruning || strings.HasPrefix(m.status, "pruned") {
			body += successStyle.Render(m.status)
		} else {
			body += m.styles.Error.Render(m.status)
		}
		body += "\n"
	}
	return panel(" Selection", body, width, height)
}

func (m Editor) renderFooter(width int) string {
	hints := []string{
		kbdPill("space", "select"),
		kbdPill("a", "all"),
		kbdPill("A", "none"),
		kbdPill("enter", "expand"),
		kbdPill("p", "prune"),
		kbdPill("esc", "cancel"),
	}
	return renderCenteredFooter(hints, width)
}

func (m Editor) visibleCount() int {
	if m.height <= 0 {
		return 10
	}
	available := m.height - 5
	if available < 4 {
		available = 4
	}
	count := available / 2
	if count < 1 {
		return 1
	}
	if count > len(m.topics) {
		return len(m.topics)
	}
	return count
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
type clearStatusMsg struct{}

func clearStatusCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{} })
}
