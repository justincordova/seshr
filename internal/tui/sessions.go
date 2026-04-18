package tui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/agentlens/internal/parser"
)

// Picker is the Session Picker Bubbletea model. See SPEC §3.1.
type Picker struct {
	metas     []parser.SessionMeta
	cursor    int
	offset    int
	width     int
	height    int
	keys      PickerKeys
	styles    Styles
	confirm   *Confirm
	deleteErr error
}

// NewPicker builds a Picker from pre-scanned metadata.
func NewPicker(metas []parser.SessionMeta) Picker {
	return Picker{
		metas:  metas,
		keys:   DefaultPickerKeys(),
		styles: NewStyles(CatppuccinMocha()),
	}
}

// Cursor returns the current selection index.
func (p Picker) Cursor() int { return p.cursor }

// InConfirm reports whether a confirm modal is currently shown.
func (p Picker) InConfirm() bool { return p.confirm != nil }

// Metas returns the current session list (post-delete).
func (p Picker) Metas() []parser.SessionMeta { return p.metas }

// Selected returns the currently highlighted SessionMeta, or the zero value
// when the list is empty.
func (p Picker) Selected() (parser.SessionMeta, bool) {
	if len(p.metas) == 0 {
		return parser.SessionMeta{}, false
	}
	return p.metas[p.cursor], true
}

// Init satisfies tea.Model.
func (p Picker) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.confirm != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			m, _ := p.confirm.Update(km)
			c := m.(Confirm)
			p.confirm = &c
			if c.Done() {
				if c.Confirmed() {
					p.deleteErr = deleteSelected(&p)
				}
				p.confirm = nil
			}
			return p, nil
		}
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, p.keys.Quit):
			return p, tea.Quit
		case key.Matches(msg, p.keys.Up):
			if p.cursor > 0 {
				p.cursor--
				p.offset = clampOffset(p.cursor, p.offset, len(p.metas), p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.Down):
			if p.cursor < len(p.metas)-1 {
				p.cursor++
				p.offset = clampOffset(p.cursor, p.offset, len(p.metas), p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.Open):
			if sel, ok := p.Selected(); ok {
				return p, func() tea.Msg { return OpenSessionMsg{Meta: sel} }
			}
			return p, nil
		case key.Matches(msg, p.keys.Delete):
			if len(p.metas) == 0 {
				return p, nil
			}
			sel := p.metas[p.cursor]
			c := NewConfirm(
				"Delete session?",
				fmt.Sprintf("%s · %s\nThis cannot be undone.", sel.Project, sel.ID),
			)
			p.confirm = &c
			return p, nil
		case key.Matches(msg, p.keys.Restore):
			if len(p.metas) == 0 {
				return p, nil
			}
			meta := p.metas[p.cursor]
			if !meta.HasBackup {
				return p, nil
			}
			return p, func() tea.Msg { return RestoreRequestedMsg{Path: meta.Path} }
		}
	}
	return p, nil
}

// View satisfies tea.Model.
func (p Picker) View() string {
	if p.confirm != nil {
		return p.confirm.View()
	}

	header := p.renderHeader()
	statsStrip := p.renderStats()
	errLine := p.renderDeleteErr()
	footer := p.renderFooter()

	fixedH := lipgloss.Height(header) + lipgloss.Height(statsStrip) +
		lipgloss.Height(errLine) + lipgloss.Height(footer)
	mainH := p.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	main := p.renderSessionPanel(mainH)

	if errLine == "" {
		return lipgloss.JoinVertical(lipgloss.Left, header, statsStrip, main, footer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, statsStrip, main, errLine, footer)
}

func (p Picker) renderDeleteErr() string {
	if p.deleteErr == nil {
		return ""
	}
	msg := lipgloss.NewStyle().Foreground(colRed).Render("delete failed: ") + p.deleteErr.Error()
	return lipgloss.NewStyle().Width(p.width).Padding(0, 2).Render(msg)
}

func renderLogo() string {
	text := "◆ AgentLens"
	gradient := []lipgloss.TerminalColor{colMauve, colMauve, colLavender, colPink, colPink, colFlamingo, colFlamingo, colPink, colPink, colLavender, colMauve}
	var b strings.Builder
	for i, r := range text {
		color := gradient[i%len(gradient)]
		b.WriteString(lipgloss.NewStyle().Foreground(color).Bold(true).Render(string(r)))
	}
	return b.String()
}

func (p Picker) renderHeader() string {
	logo := renderLogo()
	ver := dimStyle.Render("v0.1")
	left := logo + " " + ver

	right := joinHints(
		kbd("?", "help"),
	)

	gap := p.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(p.width).Padding(0, 1).Render(row)
}

func (p Picker) renderStats() string {
	var totalSize int64
	for _, m := range p.metas {
		totalSize += m.Size
	}

	items := []string{
		statInline("SESSIONS", fmt.Sprintf("%d", len(p.metas)), colGreen),
		statInline("TOTAL SIZE", humanizeSize(totalSize), colLavender),
		statInline("SOURCE", "Claude Code", colMauve),
	}

	sep := dimStyle.Render("  │  ")
	row := strings.Join(items, sep)
	return lipgloss.NewStyle().Width(p.width).Padding(0, 2).Render(row)
}

func statInline(label, value string, valueColor lipgloss.TerminalColor) string {
	l := lipgloss.NewStyle().Foreground(colOverlay1).Bold(true).Render(label)
	v := lipgloss.NewStyle().Foreground(valueColor).Bold(true).Render(value)
	return l + " " + v
}

func (p Picker) renderSessionPanel(height int) string {
	if len(p.metas) == 0 {
		body := "\n" + textStyle.Render("No sessions found.") + "\n\n" +
			dimStyle.Render("Place session JSONL files in ~/.claude/projects/")
		title := " Sessions"
		return panel(title, body, p.width, height)
	}

	title := fmt.Sprintf(" Sessions %s", dimStyle.Render(fmt.Sprintf("(%d)", len(p.metas))))
	body := p.renderSessionList(p.width - 4)
	return panel(title, body, p.width, height)
}

func panel(title, body string, width, height int) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	titleBar := ""
	if title != "" {
		titleBar = lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render(title) + "\n"
	}
	return style.Render(titleBar + body)
}

func (p Picker) renderSessionList(width int) string {
	var b strings.Builder
	visible := p.visibleCount()

	end := p.offset + visible
	if end > len(p.metas) {
		end = len(p.metas)
	}
	for i := p.offset; i < end; i++ {
		b.WriteString(p.renderSessionCard(i, width))
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (p Picker) renderSessionCard(i, width int) string {
	m := p.metas[i]
	selected := i == p.cursor

	barStyle := lipgloss.NewStyle().Foreground(colSurface1)
	if selected {
		barStyle = lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	}
	bar := barStyle.Render("▌")

	name := textStyle.Render(truncate(m.Project, 40))
	if selected {
		name = lipgloss.NewStyle().Foreground(colText).Bold(true).Render(truncate(m.Project, 40))
	}

	size := dimStyle.Render(humanizeSize(m.Size))
	age := dimStyle.Render(humanize.Time(m.ModifiedAt))

	line1Gap := width - lipgloss.Width(name) - lipgloss.Width(size) - lipgloss.Width(age) - 6
	if line1Gap < 1 {
		line1Gap = 1
	}
	line1 := bar + " " + name + strings.Repeat(" ", line1Gap) + size + "  " + age

	src := dimStyle.Render(string(m.Source))
	backup := ""
	if m.HasBackup {
		backup = dimStyle.Render(" · ") + successStyle.Render("↶ backup")
	}
	line2 := bar + "  " + src + backup

	return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
}

func (p Picker) renderFooter() string {
	hints := joinHints(
		kbd("↑↓/jk", "nav"),
		kbd("enter", "open"),
		kbd("d", "delete"),
		kbd("q", "quit"),
	)
	hintsW := lipgloss.Width(hints)
	gap := (p.width - hintsW) / 2
	if gap < 2 {
		gap = 2
	}
	return lipgloss.NewStyle().Width(p.width).Render(strings.Repeat(" ", gap) + hints)
}

// visibleCount returns the number of session cards that fit in the main panel.
// Shared between cursor-scroll clamping and list rendering.
func (p Picker) visibleCount() int {
	if p.height <= 0 {
		return 8
	}
	// View subtracts header(1) + stats(1) + footer(1) = 3 from height (min 6).
	fixedH := 3
	if p.deleteErr != nil {
		fixedH++
	}
	mainH := p.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	// Panel renders title(1) + 2 borders + 1 pad inside its Height → body = mainH-4.
	bodyH := mainH - 4
	cards := bodyH / 2 // each card renders as 2 tight lines
	if cards < 1 {
		return 1
	}
	return cards
}

// clampOffset adjusts offset so that cursor is within [offset, offset+visible).
// Scrolls only when the cursor moves past the top or bottom edge.
func clampOffset(cursor, offset, total, visible int) int {
	if total <= visible {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+visible {
		offset = cursor - visible + 1
	}
	if offset+visible > total {
		offset = total - visible
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// OpenSessionMsg is emitted by the picker when enter is pressed on a session.
type OpenSessionMsg struct {
	Meta parser.SessionMeta
}

type RestoreRequestedMsg struct{ Path string }

// deleteSelected removes the currently-highlighted session's file and
// cleans up its parent directory if it becomes empty.
func deleteSelected(p *Picker) error {
	if len(p.metas) == 0 {
		return nil
	}
	sel := p.metas[p.cursor]
	if err := os.Remove(sel.Path); err != nil {
		slog.Error("delete session failed", "path", sel.Path, "err", err)
		return err
	}
	_ = os.Remove(sel.Path + ".bak")
	_ = os.Remove(sel.Path + ".lock")
	slog.Info("deleted session", "path", sel.Path)
	dir := filepath.Dir(sel.Path)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		if rmErr := os.Remove(dir); rmErr == nil {
			slog.Info("removed empty project dir", "path", dir)
		} else {
			slog.Warn("could not remove empty project dir", "path", dir, "err", rmErr)
		}
	}
	p.metas = append(p.metas[:p.cursor], p.metas[p.cursor+1:]...)
	if p.cursor >= len(p.metas) && p.cursor > 0 {
		p.cursor--
	}
	return nil
}

func humanizeSize(n int64) string {
	return humanize.IBytes(uint64(n))
}
