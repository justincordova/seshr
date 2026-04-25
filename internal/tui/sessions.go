package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Picker is the Session Picker Bubbletea model. See SPEC §3.1.
type Picker struct {
	metas     []backend.SessionMeta
	allMetas  []backend.SessionMeta
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

	// liveIndex maps session ID → live state; updated by the app on each slow tick.
	liveIndex   map[string]*backend.LiveSession
	registry    *backend.Registry
	banner      string // set by App when live detection fails
	showWelcome bool   // show first-launch welcome
}

// NewPicker builds a Picker from pre-scanned metadata.
// reg may be nil; it is used for deletion operations.
// showWelcome displays the first-launch banner until any keypress.
func NewPicker(metas []backend.SessionMeta, th Theme, reg *backend.Registry) Picker {
	p := Picker{
		metas:     metas,
		allMetas:  metas,
		keys:      DefaultPickerKeys(),
		styles:    NewStyles(th),
		theme:     th,
		search:    NewSearchBar(),
		collapsed: make(map[string]bool),
		registry:  reg,
	}
	// Projects collapsed by default; user expands with enter or space.
	groups := GroupByProject(metas, th)
	for _, g := range groups {
		p.collapsed[g.Name] = true
	}
	p.rebuildGroups()
	return p
}

// Cursor returns the current selection index.
func (p Picker) Cursor() int { return p.cursor }

// SetLiveIndex replaces the picker's live-session map. Called by the app
// after each slow-tick reconcile so the picker rows can render pulsing
// status, current-task, and context-warning info.
//
// SetLiveIndex also synthesizes SessionMeta entries for any live sessions
// not present in the picker's existing metas (e.g. a Claude session that
// was just opened and hasn't written a transcript yet, so the launch-time
// Scan didn't see it). Synthesized rows show up at the top of their
// project group; the group is auto-expanded so the live row is visible.
func (p *Picker) SetLiveIndex(idx map[string]*backend.LiveSession) {
	p.liveIndex = idx
	p.syncLiveMetas()
}

// syncLiveMetas augments allMetas with synthesized entries for live sessions
// not already present, then rebuilds groups and auto-expands any group that
// contains a live session.
func (p *Picker) syncLiveMetas() {
	if len(p.liveIndex) == 0 {
		return
	}

	known := make(map[string]struct{}, len(p.allMetas))
	for _, m := range p.allMetas {
		known[m.ID] = struct{}{}
	}

	added := false
	for id, live := range p.liveIndex {
		if _, ok := known[id]; ok {
			continue
		}
		p.allMetas = append(p.allMetas, synthMetaFromLive(live))
		added = true
	}

	if added {
		// metas mirrors allMetas when no search filter is active.
		// (Phase 7 picker has no persistent filter that we'd clobber here.)
		p.metas = p.allMetas
		p.rebuildGroups()
	}

	// Auto-expand any group containing a live session so the row is visible.
	for _, g := range p.groups {
		for _, m := range g.Sessions {
			if _, isLive := p.liveIndex[m.ID]; isLive {
				if p.collapsed[g.Name] {
					p.collapsed[g.Name] = false
					p.rebuildGroups()
				}
				break
			}
		}
	}
}

// synthMetaFromLive builds a SessionMeta for a live session that wasn't
// found by Scan (e.g. no transcript file yet). Project is derived from CWD
// using the source's encoding rules.
func synthMetaFromLive(live *backend.LiveSession) backend.SessionMeta {
	project := projectFromLive(live)
	return backend.SessionMeta{
		ID:        live.SessionID,
		Kind:      live.Kind,
		Project:   project,
		Directory: live.CWD,
		Title:     "",
		UpdatedAt: live.LastActivity,
		CreatedAt: live.LastActivity,
	}
}

// projectFromLive returns the project-name string used for grouping. Each
// source has its own encoding (Claude uses /-replaced-paths). Falls back to
// the cwd basename or "live" when cwd is unknown.
func projectFromLive(live *backend.LiveSession) string {
	if live.Project != "" {
		return live.Project
	}
	if live.CWD == "" {
		return "live"
	}
	return live.CWD
}

// InConfirm reports whether a confirm modal is currently shown.
func (p Picker) InConfirm() bool { return p.confirm != nil }

// Metas returns the current session list (post-delete).
func (p Picker) Metas() []backend.SessionMeta { return p.metas }

// rebuildGroups recomputes the project groups and flat row index from p.metas.
func (p *Picker) rebuildGroups() {
	p.groups = GroupByProject(p.metas, p.theme)
	p.sortLiveToTop()
	p.flatRows = BuildFlatRows(p.groups, p.collapsed)
}

func (p *Picker) sortLiveToTop() {
	if len(p.liveIndex) == 0 {
		return
	}
	for i := range p.groups {
		g := &p.groups[i]
		sort.SliceStable(g.Sessions, func(a, b int) bool {
			aLive := p.liveIndex[g.Sessions[a].ID] != nil
			bLive := p.liveIndex[g.Sessions[b].ID] != nil
			if aLive != bLive {
				return aLive
			}
			return g.Sessions[a].UpdatedAt.After(g.Sessions[b].UpdatedAt)
		})
	}
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
func (p Picker) selectedSession() (backend.SessionMeta, bool) {
	row, ok := p.selectedRow()
	if !ok || row.Kind != RowSession {
		return backend.SessionMeta{}, false
	}
	return p.groups[row.GroupIdx].Sessions[row.SessionIdx], true
}

// Selected returns the currently highlighted SessionMeta, or the zero value
// when the list is empty or cursor is on a group header.
func (p Picker) Selected() (backend.SessionMeta, bool) {
	return p.selectedSession()
}

// Init satisfies tea.Model.
func (p Picker) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Dismiss welcome banner on any keypress.
	if p.showWelcome {
		if _, ok := msg.(tea.KeyMsg); ok {
			p.showWelcome = false
			// TODO: persist config.WelcomeShown = true via a command.
		}
	}

	if p.confirm != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			m, _ := p.confirm.Update(km)
			c := m.(Confirm)
			p.confirm = &c
			if c.Done() {
				if c.Confirmed() {
					p.deleteErr = deleteSelected(&p, p.registry)
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
		case key.Matches(msg, p.keys.Delete):
			sel, ok := p.selectedSession()
			if !ok {
				return p, nil
			}
			row, _ := p.selectedRow()
			g := p.groups[row.GroupIdx]
			c := NewConfirm(
				"Delete session?",
				fmt.Sprintf("%s · %s\nThis cannot be undone.", g.DisplayName, sel.Project),
				p.theme,
			)
			p.confirm = &c
			return p, nil
		case key.Matches(msg, p.keys.Restore):
			sel, ok := p.selectedSession()
			if !ok || !sel.HasBackup {
				return p, nil
			}
			return p, func() tea.Msg { return RestoreRequestedMsg{ID: sel.ID, Kind: sel.Kind} }
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
		return lipgloss.Place(p.width, p.height, lipgloss.Center, lipgloss.Center, p.confirm.View())
	}

	cw := contentWidth(p.width)

	header := p.renderHeader(cw)
	welcomeLine := p.renderWelcome(cw)
	statsStrip := p.renderStats(cw)
	bannerLine := p.renderBanner(cw)
	errLine := p.renderDeleteErr(cw)
	searchBar := p.search.View(cw)
	footer := p.renderFooter(cw)

	fixedH := lipgloss.Height(header) + lipgloss.Height(welcomeLine) +
		lipgloss.Height(statsStrip) + lipgloss.Height(bannerLine) +
		lipgloss.Height(errLine) + lipgloss.Height(searchBar) + lipgloss.Height(footer)

	mainH := p.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	main := p.renderSessionPanel(cw, mainH)

	parts := []string{header}
	if welcomeLine != "" {
		parts = append(parts, welcomeLine)
	}
	parts = append(parts, statsStrip)
	if bannerLine != "" {
		parts = append(parts, bannerLine)
	}
	parts = append(parts, main)
	if errLine != "" {
		parts = append(parts, errLine)
	}
	if searchBar != "" {
		parts = append(parts, searchBar)
	}
	parts = append(parts, footer)
	return centerBlock(lipgloss.JoinVertical(lipgloss.Left, parts...), p.width)
}

func (p Picker) renderWelcome(width int) string {
	if !p.showWelcome {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(colSubtext0).
		Width(width).
		Padding(0, 2).
		Render("Welcome to seshr. Select a session to open, or press ? for help.")
}

func (p Picker) renderBanner(width int) string {
	if p.banner == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(colSubtext0).
		Width(width).
		Padding(0, 2).
		Render("  " + p.banner)
}

func (p Picker) renderDeleteErr(width int) string {
	if p.deleteErr == nil {
		return ""
	}
	msg := lipgloss.NewStyle().Foreground(colRed).Render("delete failed: ") + p.deleteErr.Error()
	return lipgloss.NewStyle().Width(width).Padding(0, 2).Render(msg)
}

func renderLogo() string {
	text := "◆ Seshr"
	gradient := []lipgloss.TerminalColor{colMauve, colMauve, colLavender, colPink, colPink, colFlamingo, colFlamingo, colPink, colPink, colLavender, colMauve}
	var b strings.Builder
	for i, r := range text {
		color := gradient[i%len(gradient)]
		b.WriteString(lipgloss.NewStyle().Foreground(color).Bold(true).Render(string(r)))
	}
	return b.String()
}

func (p Picker) renderHeader(width int) string {
	logo := renderLogo()
	ver := dimStyle.Render("v0.1")
	row := logo + " " + ver
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(row)
}

func (p Picker) renderStats(width int) string {
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
	}
	if liveCount := len(p.liveIndex); liveCount > 0 {
		items = append(items, statItem{"LIVE", fmt.Sprintf("%d", liveCount)})
	}
	items = append(items,
		statItem{"PROJECTS", fmt.Sprintf("%d", sum.Projects)},
		statItem{"TOKENS", humanize.Comma(sum.TotalTokens)},
		statItem{"SIZE", humanizeSize(sum.TotalBytes)},
		statItem{"LATEST", humanize.Time(sum.MostRecent)},
	)

	var parts []string
	for i, it := range items {
		color := accents[i%len(accents)]
		parts = append(parts, statInline(it.label, it.value, color))
	}

	sep := dimStyle.Render(" · ")
	row := strings.Join(parts, sep)

	rowWidth := lipgloss.Width(row)
	maxWidth := width - 4
	for rowWidth > maxWidth && len(parts) > 2 {
		parts = parts[:len(parts)-1]
		row = strings.Join(parts, sep)
		rowWidth = lipgloss.Width(row)
	}

	return lipgloss.NewStyle().Width(width).Padding(0, 2).Render(row)
}

func statInline(label, value string, valueColor lipgloss.TerminalColor) string {
	l := lipgloss.NewStyle().Foreground(colOverlay1).Bold(true).Render(label)
	v := lipgloss.NewStyle().Foreground(valueColor).Bold(true).Render(value)
	return l + " " + v
}

func (p Picker) renderSessionPanel(width, height int) string {
	if len(p.metas) == 0 {
		body := "\n" + textStyle.Render("No sessions found.") + "\n\n" +
			dimStyle.Render("Place session JSONL files in ~/.claude/projects/")
		title := " Sessions"
		return panel(title, body, width, height)
	}

	body := p.renderGroupedList(width - 4)
	return panel("", body, width, height)
}

func (p Picker) renderGroupedList(width int) string {
	var b strings.Builder
	// Each row is exactly one line (see rowHeight). Lines are separated by
	// a single '\n' which costs no extra budget — the panel body is sized
	// to fit `budget` lines verbatim. Group boundaries are visually clear
	// from the project gutter color and uppercase project header, so we
	// don't insert blank separator rows.
	budget := p.bodyLines()

	linesUsed := 0
	for i := p.offset; i < len(p.flatRows) && linesUsed < budget; i++ {
		if linesUsed > 0 {
			b.WriteByte('\n')
		}
		row := p.flatRows[i]
		switch row.Kind {
		case RowGroup:
			b.WriteString(p.renderGroupHeader(row, i == p.cursor, width))
		case RowSession:
			meta := p.groups[row.GroupIdx].Sessions[row.SessionIdx]
			g := p.groups[row.GroupIdx]
			b.WriteString(p.renderSessionRow(meta, g.Color, i == p.cursor, width))
		}
		linesUsed++
	}
	return b.String()
}

func panel(title, body string, width, height int) string {
	if title == "" {
		style := boxStyle.Width(width - 2).Height(height - 2)
		return style.Render(body)
	}
	style := boxStyle.Width(width - 2).Height(height - 3)
	titleBar := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Width(width - 4).Render(title)
	return titleBar + "\n" + style.Render(body)
}

func (p Picker) renderGroupHeader(row PickerRow, selected bool, width int) string {
	g := p.groups[row.GroupIdx]
	isCollapsed := p.collapsed[g.Name]

	glyph := "▾"
	if isCollapsed {
		glyph = "▸"
	}

	gutterStyle := lipgloss.NewStyle().Foreground(g.Color).Bold(true)
	if !selected {
		gutterStyle = lipgloss.NewStyle().Foreground(g.Color).Faint(true)
	}
	gutter := gutterStyle.Render("▌")

	// Project name in its own color + bold + uppercase so each project reads
	// as a distinct visual anchor. Selection just brightens via no-faint.
	nameStyle := lipgloss.NewStyle().Foreground(g.Color).Bold(true)
	if !selected {
		nameStyle = lipgloss.NewStyle().Foreground(g.Color).Bold(true).Faint(true)
	}
	name := nameStyle.Render(strings.ToUpper(truncate(g.DisplayName, 30)))

	count := dimStyle.Render(fmt.Sprintf("%s %s", glyph, countLabel(len(g.Sessions), "session")))
	tokStr := dimStyle.Render(humanizeTokens(int64(g.TotalTokens)) + " tok")

	left := gutter + " " + name
	right := count + "  " + tokStr
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 4 {
		gap = 4
	}
	line1 := left + strings.Repeat(" ", gap) + right
	return line1
}

func (p Picker) renderSessionRow(m backend.SessionMeta, projectColor lipgloss.TerminalColor, selected bool, width int) string {
	live := p.liveIndex[m.ID]

	// Gutter
	gutterStyle := lipgloss.NewStyle().Foreground(projectColor).Bold(true)
	if !selected {
		gutterStyle = lipgloss.NewStyle().Foreground(projectColor).Faint(true)
	}
	gutter := gutterStyle.Render("▌")

	// Glyph: live vs ended
	var glyph string
	if live != nil {
		switch live.Status {
		case backend.StatusWorking:
			glyph = lipgloss.NewStyle().Foreground(p.theme.Success).Bold(true).Render("●")
		case backend.StatusWaiting:
			glyph = lipgloss.NewStyle().Foreground(p.theme.Warning).Bold(true).Render("●")
		default:
			glyph = lipgloss.NewStyle().Foreground(colOverlay1).Render("◌")
		}
		if live.Ambiguous {
			glyph = lipgloss.NewStyle().Foreground(colOverlay1).Render("◌")
		}
	} else {
		glyphStyle := lipgloss.NewStyle().Foreground(colOverlay1)
		if selected {
			glyphStyle = lipgloss.NewStyle().Foreground(p.theme.Accent).Bold(true)
		}
		glyph = glyphStyle.Render("▸")
	}

	// ID
	idStyle := lipgloss.NewStyle().Foreground(colOverlay1)
	if selected {
		idStyle = lipgloss.NewStyle().Foreground(p.theme.Foreground)
	}
	id := idStyle.Render(truncate(m.ID, 20))

	// Source badge (fixed-width dim)
	badgeStyle := lipgloss.NewStyle().Foreground(colSubtext0)
	badge := badgeStyle.Render(sourceBadge(m.Kind))

	sessMetaStyle := lipgloss.NewStyle().Foreground(colSubtext0)
	tokStr := sessMetaStyle.Render(humanizeTokens(int64(m.TokenCount)))

	backup := ""
	if m.HasBackup {
		backup = "  " + successStyle.Render("↶")
	}

	// Right side: live status or age
	var statusStr string
	if live != nil {
		switch {
		case live.Ambiguous:
			statusStr = sessMetaStyle.Render("? live")
		case live.Status == backend.StatusWorking && live.CurrentTask != "":
			statusStr = lipgloss.NewStyle().Foreground(p.theme.Success).Render("working · " + truncate(live.CurrentTask, 30))
		case live.Status == backend.StatusWorking:
			statusStr = lipgloss.NewStyle().Foreground(p.theme.Success).Render("working")
		default:
			statusStr = lipgloss.NewStyle().Foreground(p.theme.Warning).Render("waiting")
		}
		// Append context warning if ≥ 80%.
		if live.ContextTokens > 0 && live.ContextWindow > 0 {
			pct := live.ContextTokens * 100 / live.ContextWindow
			if pct >= 80 {
				ctxWarn := lipgloss.NewStyle().Foreground(p.theme.Warning).Render(fmt.Sprintf("ctx %d%% ⚠", pct))
				statusStr += " · " + ctxWarn
			}
		}
	}

	left := gutter + "   " + glyph + " " + id
	if width >= 100 {
		// Full layout: badge + tok + status/age.
		var right string
		if live != nil {
			right = badge + "  " + tokStr + "  " + statusStr + backup
		} else {
			age := sessMetaStyle.Render(humanize.Time(m.UpdatedAt))
			right = badge + "  " + tokStr + "  " + age + backup
		}
		gap := width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		return left + strings.Repeat(" ", gap) + right
	}
	if width >= 80 {
		// Compact: drop badge column, keep tok + status/age.
		var right string
		if live != nil {
			right = tokStr + "  " + statusStr + backup
		} else {
			age := sessMetaStyle.Render(humanize.Time(m.UpdatedAt))
			right = tokStr + "  " + age + backup
		}
		gap := width - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		return left + strings.Repeat(" ", gap) + right
	}
	// Narrow: just id + tok, task truncated to 20.
	var right string
	if live != nil {
		shortTask := truncate(live.CurrentTask, 20)
		if shortTask != "" {
			right = tokStr + "  " + sessMetaStyle.Render(shortTask) + backup
		} else {
			right = tokStr + backup
		}
	} else {
		right = tokStr + backup
	}
	return gutter + "   " + glyph + " " + id + "  " + right
}

// sourceBadge returns the fixed-width source name for a session's source kind.
func sourceBadge(kind session.SourceKind) string {
	switch kind {
	case session.SourceClaude:
		return "claude  "
	case session.SourceOpenCode:
		return "opencode"
	default:
		return string(kind)
	}
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
		p.metas = make([]backend.SessionMeta, 0, len(p.search.Matches()))
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

func (p Picker) renderFooter(width int) string {
	hints := []string{
		kbdPill("↑↓/jk", "nav"),
		kbdPill("enter", "open"),
		kbdPill("d", "delete"),
		kbdPill("/", "search"),
		kbdPill("q", "quit"),
	}
	return renderCenteredFooter(hints, width)
}

// bodyLines returns the number of text lines available inside the main
// session panel's body (inside the border chrome). Mirrors the chrome
// arithmetic in View() so key handlers (which don't have access to the
// computed mainH) get the same answer.
//
// Chrome stack (each contributes its rendered Height):
//
//	header                      — always 1 line
//	welcome banner              — 1 line when showWelcome
//	stats strip                 — always 1 line
//	live-detection banner       — 1 line when banner != ""
//	main panel (this)           — uses the remaining vertical
//	delete-error line           — 1 line when deleteErr != nil
//	search bar                  — 1 line when search is open
//	footer                      — always 1 line
//
// boxStyle adds a 1-line top + 1-line bottom border around the panel body
// (no vertical padding), so we subtract another 2 to get the body budget.
func (p Picker) bodyLines() int {
	if p.height <= 0 {
		return 16
	}
	fixedH := 3 // header + stats + footer
	if p.showWelcome {
		fixedH++
	}
	if p.banner != "" {
		fixedH++
	}
	if p.deleteErr != nil {
		fixedH++
	}
	if p.search.Active() {
		fixedH++
	}
	mainH := p.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	bodyH := mainH - 2 // border top + bottom
	if bodyH < 1 {
		return 1
	}
	return bodyH
}

// visibleCount returns the row count used for cursor-scroll clamping. All
// rows are 1 line tall (rowHeight always returns 1), so this equals the
// body line budget.
func (p Picker) visibleCount() int {
	return p.bodyLines()
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
	Meta backend.SessionMeta
}

type OpenSessionAndReplayMsg struct {
	Meta backend.SessionMeta
}

type RestoreRequestedMsg struct {
	ID   string
	Kind session.SourceKind
}

// deleteSelected removes the currently-highlighted session via the registry editor.
func deleteSelected(p *Picker, reg *backend.Registry) error {
	sel, ok := p.selectedSession()
	if !ok {
		return nil
	}
	if reg != nil {
		ed, ok := reg.Editor(sel.Kind)
		if ok {
			ctx := context.Background()
			if err := ed.Delete(ctx, sel.ID); err != nil {
				slog.Error("delete session failed", "id", sel.ID, "err", err)
				return err
			}
			slog.Info("deleted session", "id", sel.ID)
		}
	}
	p.allMetas = removeMeta(p.allMetas, sel.ID)
	p.metas = removeMeta(p.metas, sel.ID)
	p.rebuildGroups()
	if p.cursor >= len(p.flatRows) && p.cursor > 0 {
		p.cursor--
	}
	return nil
}

func removeMeta(metas []backend.SessionMeta, id string) []backend.SessionMeta {
	for i, m := range metas {
		if m.ID == id {
			return append(metas[:i], metas[i+1:]...)
		}
	}
	return metas
}

func humanizeSize(n int64) string {
	return humanize.IBytes(uint64(n))
}

func humanizeTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
