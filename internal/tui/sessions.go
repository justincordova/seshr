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
	allMetas  []parser.SessionMeta
	cursor    int
	offset    int
	width     int
	height    int
	keys      PickerKeys
	styles    Styles
	theme     Theme
	confirm   *Confirm
	deleteErr error
	search    SearchBar

	groups    []ProjectGroup
	collapsed map[string]bool
	flatRows  []PickerRow
}

// NewPicker builds a Picker from pre-scanned metadata.
func NewPicker(metas []parser.SessionMeta, th Theme) Picker {
	p := Picker{
		metas:     metas,
		allMetas:  metas,
		keys:      DefaultPickerKeys(),
		styles:    NewStyles(th),
		theme:     th,
		search:    NewSearchBar(),
		collapsed: make(map[string]bool),
	}
	p.rebuildGroups()
	return p
}

// Cursor returns the current selection index.
func (p Picker) Cursor() int { return p.cursor }

// InConfirm reports whether a confirm modal is currently shown.
func (p Picker) InConfirm() bool { return p.confirm != nil }

// Metas returns the current session list (post-delete).
func (p Picker) Metas() []parser.SessionMeta { return p.metas }

// rebuildGroups recomputes the project groups and flat row index from p.metas.
func (p *Picker) rebuildGroups() {
	p.groups = GroupByProject(p.metas, p.theme)
	p.flatRows = BuildFlatRows(p.groups, p.collapsed)
}

func (p *Picker) toggleCollapse(row PickerRow) {
	g := p.groups[row.GroupIdx]
	p.collapsed[g.Name] = !p.collapsed[g.Name]
	p.rebuildGroups()
	if p.cursor >= len(p.flatRows) {
		p.cursor = len(p.flatRows) - 1
	}
}

// selectedRow returns the flat row at the cursor, or false if no rows.
func (p Picker) selectedRow() (PickerRow, bool) {
	if len(p.flatRows) == 0 || p.cursor >= len(p.flatRows) {
		return PickerRow{}, false
	}
	return p.flatRows[p.cursor], true
}

// selectedSession returns the session meta at the cursor, or false if the
// cursor is on a group header or there are no rows.
func (p Picker) selectedSession() (parser.SessionMeta, bool) {
	row, ok := p.selectedRow()
	if !ok || row.Kind != RowSession {
		return parser.SessionMeta{}, false
	}
	return p.groups[row.GroupIdx].Sessions[row.SessionIdx], true
}

// Selected returns the currently highlighted SessionMeta, or the zero value
// when the list is empty or cursor is on a group header.
func (p Picker) Selected() (parser.SessionMeta, bool) {
	return p.selectedSession()
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
		if p.search.Active() {
			switch msg.String() {
			case "esc":
				p.search.Close()
				p.metas = p.allMetas
				p.rebuildGroups()
				if p.cursor >= len(p.flatRows) {
					p.cursor = len(p.flatRows) - 1
				}
				if p.cursor < 0 {
					p.cursor = 0
				}
				p.offset = clampOffset(p.cursor, p.offset, len(p.flatRows), p.visibleCount())
				return p, nil
			case "enter":
				p.search.Commit()
				p.applySearchFilter()
				return p, nil
			case "up", "ctrl+p":
				if len(p.flatRows) > 0 && p.cursor > 0 {
					p.cursor--
				}
				return p, nil
			case "down", "ctrl+n":
				if p.cursor < len(p.flatRows)-1 {
					p.cursor++
				}
				return p, nil
			default:
				p.search.Update(msg)
				p.applySearchFilter()
				return p, nil
			}
		}
		switch {
		case key.Matches(msg, p.keys.Quit):
			return p, tea.Quit
		case key.Matches(msg, p.keys.Search):
			p.search.Open()
			return p, nil
		case key.Matches(msg, p.keys.Up):
			if p.cursor > 0 {
				p.cursor--
				p.offset = clampOffset(p.cursor, p.offset, len(p.flatRows), p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.Down):
			if p.cursor < len(p.flatRows)-1 {
				p.cursor++
				p.offset = clampOffset(p.cursor, p.offset, len(p.flatRows), p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.Open):
			if sel, ok := p.selectedSession(); ok {
				return p, func() tea.Msg { return OpenSessionMsg{Meta: sel} }
			}
			if row, ok := p.selectedRow(); ok && row.Kind == RowGroup {
				p.toggleCollapse(row)
			}
			return p, nil
		case key.Matches(msg, p.keys.Replay):
			if sel, ok := p.selectedSession(); ok {
				return p, func() tea.Msg { return OpenSessionAndReplayMsg{Meta: sel} }
			}
			return p, nil
		case key.Matches(msg, p.keys.Edit):
			if sel, ok := p.selectedSession(); ok {
				return p, func() tea.Msg { return OpenSessionAndEditMsg{Meta: sel} }
			}
			return p, nil
		case key.Matches(msg, p.keys.Delete):
			sel, ok := p.selectedSession()
			if !ok {
				return p, nil
			}
			c := NewConfirm(
				"Delete session?",
				fmt.Sprintf("%s · %s\nThis cannot be undone.", sel.Project, sel.ID),
			)
			p.confirm = &c
			return p, nil
		case key.Matches(msg, p.keys.Restore):
			sel, ok := p.selectedSession()
			if !ok || !sel.HasBackup {
				return p, nil
			}
			return p, func() tea.Msg { return RestoreRequestedMsg{Path: sel.Path} }
		}
		if msg.String() == " " {
			if row, ok := p.selectedRow(); ok && row.Kind == RowGroup {
				p.toggleCollapse(row)
				return p, nil
			}
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
	searchBar := p.search.View(p.width)
	footer := p.renderFooter()

	fixedH := lipgloss.Height(header) + lipgloss.Height(statsStrip) +
		lipgloss.Height(errLine) + lipgloss.Height(searchBar) + lipgloss.Height(footer)

	detailH := 0
	if len(p.flatRows) > 0 {
		detailH = 5
	}

	mainH := p.height - fixedH - detailH
	if mainH < 6 {
		mainH = 6
	}
	main := p.renderSessionPanel(mainH)

	parts := []string{header, statsStrip, main}
	if detailH > 0 {
		parts = append(parts, p.renderDetailPanel())
	}
	if errLine != "" {
		parts = append(parts, errLine)
	}
	if searchBar != "" {
		parts = append(parts, searchBar)
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

const detailPanelHeight = 5

func (p Picker) renderDetailPanel() string {
	row, ok := p.selectedRow()
	if !ok {
		return ""
	}

	var body string
	switch row.Kind {
	case RowGroup:
		g := p.groups[row.GroupIdx]
		body = p.renderGroupDetail(g)
	case RowSession:
		m := p.groups[row.GroupIdx].Sessions[row.SessionIdx]
		g := p.groups[row.GroupIdx]
		body = p.renderSessionDetail(m, g.Name)
	}

	return panel(" Detail", body, p.width, detailPanelHeight)
}

func (p Picker) renderGroupDetail(g ProjectGroup) string {
	title := lipgloss.NewStyle().Foreground(g.Color).Bold(true).Render(truncate(g.Name, 50))
	count := dimStyle.Render(countLabel(len(g.Sessions), "session"))
	tokens := dimStyle.Render(humanize.Comma(int64(g.TotalTokens)) + " tokens total")

	return "\n" + title + dimStyle.Render(" · ") + count + dimStyle.Render(" · ") + tokens + "\n"
}

func (p Picker) renderSessionDetail(m parser.SessionMeta, projectName string) string {
	title := lipgloss.NewStyle().Foreground(p.theme.Accent).Bold(true).Render(
		truncate(projectName, 20) + dimStyle.Render("/") + truncate(m.ID, 30),
	)

	pct := ContextPct(m.TokenCount, contextWindow)
	bar := ContextBar(pct, 20, p.theme)
	pctStr := fmt.Sprintf("%.0f%% of 200k", pct*100)
	tokenLine := humanize.Comma(int64(m.TokenCount)) + " tokens  " + bar + "  " + pctStr

	backup := ""
	if m.HasBackup {
		backup = dimStyle.Render(" · ") + successStyle.Render("backup available")
	}
	metaLine := dimStyle.Render("modified "+humanize.Time(m.ModifiedAt)) +
		dimStyle.Render(" · ") + dimStyle.Render(humanizeSize(m.Size)) + backup

	if m.TurnCount > 0 {
		metaLine += dimStyle.Render(" · ") + dimStyle.Render(fmt.Sprintf("%d turns", m.TurnCount))
	}

	return "\n" + title + "\n" + dimStyle.Render(tokenLine) + "\n" + metaLine + "\n"
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

	right := dimStyle.Render("↑↓ select · enter open · q quit")

	gap := p.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(p.width).Padding(0, 1).Render(row)
}

func (p Picker) renderStats() string {
	sum := ComputeSummary(p.metas)
	accents := p.theme.ProjectPalette
	if len(accents) == 0 {
		accents = []lipgloss.AdaptiveColor{colMauve, colBlue, colGreen, colLavender, colPink}
	}

	type statItem struct {
		label string
		value string
	}
	items := []statItem{
		{"SESSIONS", fmt.Sprintf("%d", sum.TotalSessions)},
		{"PROJECTS", fmt.Sprintf("%d", sum.Projects)},
		{"TOKENS", humanize.Comma(sum.TotalTokens)},
		{"SIZE", humanizeSize(sum.TotalBytes)},
		{"LATEST", humanize.Time(sum.MostRecent)},
	}

	var parts []string
	for i, it := range items {
		color := accents[i%len(accents)]
		parts = append(parts, pill(it.label+" "+it.value, p.theme.Foreground, color))
	}

	sep := dimStyle.Render("  ")
	row := strings.Join(parts, sep)
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
	body := p.renderGroupedList(p.width - 4)
	return panel(title, body, p.width, height)
}

func (p Picker) renderGroupedList(width int) string {
	var b strings.Builder
	visible := p.visibleCount()

	end := p.offset + visible
	if end > len(p.flatRows) {
		end = len(p.flatRows)
	}
	for i := p.offset; i < end; i++ {
		row := p.flatRows[i]
		switch row.Kind {
		case RowGroup:
			b.WriteString(p.renderGroupHeader(row, i == p.cursor, width))
		case RowSession:
			meta := p.groups[row.GroupIdx].Sessions[row.SessionIdx]
			g := p.groups[row.GroupIdx]
			b.WriteString(p.renderSessionRow(meta, g.Color, i == p.cursor, width))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func panel(title, body string, width, height int) string {
	style := boxStyle.Width(width - 2).Height(height - 2)
	titleBar := ""
	if title != "" {
		titleBar = lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render(title) + "\n"
	}
	return style.Render(titleBar + body)
}

func (p Picker) renderGroupHeader(row PickerRow, selected bool, width int) string {
	g := p.groups[row.GroupIdx]
	isCollapsed := p.collapsed[g.Name]

	glyph := "▾"
	if isCollapsed {
		glyph = "▸"
	}

	gutterStyle := lipgloss.NewStyle().Foreground(g.Color)
	if !selected {
		gutterStyle = lipgloss.NewStyle().Foreground(g.Color).Faint(true)
	}
	gutter := gutterStyle.Render("▌")

	nameStyle := lipgloss.NewStyle().Foreground(p.theme.Foreground).Bold(true)
	if !selected {
		nameStyle = lipgloss.NewStyle().Foreground(colOverlay1)
	}
	name := nameStyle.Render(truncate(g.Name, 30))

	count := dimStyle.Render(fmt.Sprintf("%s %s", glyph, countLabel(len(g.Sessions), "session")))
	tokens := dimStyle.Render(humanize.Comma(int64(g.TotalTokens)) + " tok")

	gap := width - lipgloss.Width(gutter) - lipgloss.Width(name) - lipgloss.Width(count) - lipgloss.Width(tokens) - 4
	if gap < 1 {
		gap = 1
	}
	return gutter + " " + name + strings.Repeat(" ", gap) + count + "  " + tokens
}

func (p Picker) renderSessionRow(m parser.SessionMeta, projectColor lipgloss.TerminalColor, selected bool, width int) string {
	indent := "  "

	glyphStyle := lipgloss.NewStyle().Foreground(colOverlay1)
	if selected {
		glyphStyle = lipgloss.NewStyle().Foreground(p.theme.Accent).Bold(true)
	}
	glyph := glyphStyle.Render("▸")

	idStyle := lipgloss.NewStyle().Foreground(colOverlay1)
	if selected {
		idStyle = lipgloss.NewStyle().Foreground(p.theme.Foreground)
	}
	id := idStyle.Render(truncate(m.ID, 20))

	pct := ContextPct(m.TokenCount, contextWindow)
	bar := ContextBar(pct, 8, p.theme)
	pctStr := dimStyle.Render(fmt.Sprintf("%.0f%%", pct*100))

	tokStr := dimStyle.Render(humanize.Comma(int64(m.TokenCount)) + " tok")
	age := dimStyle.Render(humanize.Time(m.ModifiedAt))

	backup := ""
	if m.HasBackup {
		backup = "  " + successStyle.Render("↶")
	}

	row := indent + glyph + " " + id + " " + bar + " " + pctStr + "  " + tokStr + "  " + age + backup
	return row
}

func (p *Picker) applySearchFilter() {
	if p.search.Query() == "" {
		p.metas = p.allMetas
	} else {
		haystack := make([]string, len(p.allMetas))
		for i, m := range p.allMetas {
			haystack[i] = m.Project + " " + m.ID
		}
		p.search.Filter(haystack)
		p.metas = make([]parser.SessionMeta, 0, len(p.search.Matches()))
		for _, m := range p.search.Matches() {
			p.metas = append(p.metas, p.allMetas[m.Index])
		}
	}
	p.rebuildGroups()
	if p.cursor >= len(p.flatRows) {
		p.cursor = len(p.flatRows) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.offset = clampOffset(p.cursor, p.offset, len(p.flatRows), p.visibleCount())
}

func (p Picker) renderFooter() string {
	hints := joinHints(
		kbdPill("↑↓/jk", "nav"),
		kbdPill("enter", "open"),
		kbdPill("r", "replay"),
		kbdPill("e", "edit"),
		kbdPill("space", "collapse"),
		kbdPill("d", "delete"),
		kbdPill("/", "search"),
		kbdPill("q", "quit"),
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
	fixedH := 3
	if p.deleteErr != nil {
		fixedH++
	}
	mainH := p.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	bodyH := mainH - 4
	if bodyH < 1 {
		return 1
	}
	return bodyH
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

type OpenSessionAndReplayMsg struct {
	Meta parser.SessionMeta
}

type OpenSessionAndEditMsg struct {
	Meta parser.SessionMeta
}

type RestoreRequestedMsg struct{ Path string }

// deleteSelected removes the currently-highlighted session's file and
// cleans up its parent directory if it becomes empty.
func deleteSelected(p *Picker) error {
	sel, ok := p.selectedSession()
	if !ok {
		return nil
	}
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
	p.allMetas = removeMeta(p.allMetas, sel.Path)
	p.metas = removeMeta(p.metas, sel.Path)
	p.rebuildGroups()
	if p.cursor >= len(p.flatRows) && p.cursor > 0 {
		p.cursor--
	}
	return nil
}

func removeMeta(metas []parser.SessionMeta, path string) []parser.SessionMeta {
	for i, m := range metas {
		if m.Path == path {
			return append(metas[:i], metas[i+1:]...)
		}
	}
	return metas
}

func humanizeSize(n int64) string {
	return humanize.IBytes(uint64(n))
}
