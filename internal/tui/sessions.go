package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/config"
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

	// viewMode is one of config.PickerViewRecent or config.PickerViewProject.
	// Recent (default): flat list with live sessions pinned at top + divider.
	// Project: groups-by-project with synthetic "LIVE" group pinned at top.
	viewMode string

	// recentMetas is the ordered list of metas used by Recent view; live
	// rows first (sorted by status class then UpdatedAt desc), ended rows
	// after. Indexed by PickerRow.SessionIdx when viewMode == Recent.
	recentMetas []backend.SessionMeta
}

// NewPicker builds a Picker from pre-scanned metadata.
// reg may be nil; it is used for deletion operations.
// viewMode is one of config.PickerViewRecent or config.PickerViewProject;
// any other value falls back to Recent.
func NewPicker(metas []backend.SessionMeta, th Theme, reg *backend.Registry, viewMode string) Picker {
	if viewMode != config.PickerViewProject {
		viewMode = config.PickerViewRecent
	}
	p := Picker{
		metas:     metas,
		allMetas:  metas,
		keys:      DefaultPickerKeys(),
		styles:    NewStyles(th),
		theme:     th,
		search:    NewSearchBar(),
		collapsed: make(map[string]bool),
		registry:  reg,
		viewMode:  viewMode,
	}
	// Projects collapsed by default; user expands with enter or space.
	// (Has no effect in Recent view, but cheap to compute.)
	groups := GroupByProject(metas, th)
	for _, g := range groups {
		p.collapsed[g.Name] = true
	}
	p.rebuildGroups()
	return p
}

// Cursor returns the current selection index.
func (p Picker) Cursor() int { return p.cursor }

// ViewMode returns the picker's current view mode (config.PickerViewRecent
// or config.PickerViewProject).
func (p Picker) ViewMode() string { return p.viewMode }

// SetLiveIndex replaces the picker's live-session map. Called by the app
// after each slow-tick reconcile so the picker rows can render pulsing
// status, current-task, and context-warning info.
//
// SetLiveIndex also synthesizes SessionMeta entries for any live sessions
// not present in the picker's existing metas (e.g. a Claude session that
// was just opened and hasn't written a transcript yet, so the launch-time
// Scan didn't see it). The new index is then folded into the row layout —
// in Project view this materializes the synthetic LIVE pinned group; in
// Recent view it pins live sessions to the top of the flat list.
func (p *Picker) SetLiveIndex(idx map[string]*backend.LiveSession) {
	p.liveIndex = idx
	p.syncLiveMetas()
	// liveIndex changes affect both Recent and Project layouts (LIVE group
	// presence, live-row pinning, divider). Always rebuild so the next
	// View() reflects current liveness.
	p.rebuildGroups()
}

// syncLiveMetas augments allMetas with synthesized entries for live sessions
// not already present in the metas list (e.g. a process detected before its
// transcript exists on disk).
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
		p.metas = p.allMetas
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

// rebuildGroups recomputes the flat row index from p.metas. In Recent mode
// it builds a flat list with live rows pinned at top + divider; in Project
// mode it builds project groups and a synthetic LIVE group at the top
// (live sessions already sorted by status class inside SplitLiveGroup).
func (p *Picker) rebuildGroups() {
	if p.viewMode == config.PickerViewRecent {
		p.flatRows, p.recentMetas = BuildRecentRows(p.metas, p.liveIndex)
		p.groups = nil
		return
	}
	p.recentMetas = nil
	p.groups = GroupByProject(p.metas, p.theme)
	p.groups = SplitLiveGroup(p.groups, p.liveIndex, p.theme)
	// LIVE group always renders expanded; clear any stale collapsed flag.
	if len(p.groups) > 0 && p.groups[0].Name == liveGroupName {
		delete(p.collapsed, liveGroupName)
	}
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
// cursor is on a group header, a divider, or there are no rows.
func (p Picker) selectedSession() (backend.SessionMeta, bool) {
	row, ok := p.selectedRow()
	if !ok || row.Kind != RowSession {
		return backend.SessionMeta{}, false
	}
	if p.viewMode == config.PickerViewRecent {
		if row.SessionIdx < 0 || row.SessionIdx >= len(p.recentMetas) {
			return backend.SessionMeta{}, false
		}
		return p.recentMetas[row.SessionIdx], true
	}
	if row.GroupIdx < 0 || row.GroupIdx >= len(p.groups) {
		return backend.SessionMeta{}, false
	}
	g := p.groups[row.GroupIdx]
	if row.SessionIdx < 0 || row.SessionIdx >= len(g.Sessions) {
		return backend.SessionMeta{}, false
	}
	return g.Sessions[row.SessionIdx], true
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
		case key.Matches(msg, p.keys.Top):
			p.cursor = 0
			p.offset = 0
			return p, nil
		case key.Matches(msg, p.keys.Bottom):
			if n := len(p.flatRows); n > 0 {
				p.cursor = n - 1
				p.offset = clampOffset(p.cursor, p.offset, n, p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.PageDown):
			if n := len(p.flatRows); n > 0 {
				step := p.visibleCount() / 2
				if step < 1 {
					step = 1
				}
				p.cursor += step
				if p.cursor > n-1 {
					p.cursor = n - 1
				}
				p.offset = clampOffset(p.cursor, p.offset, n, p.visibleCount())
			}
			return p, nil
		case key.Matches(msg, p.keys.PageUp):
			if len(p.flatRows) > 0 {
				step := p.visibleCount() / 2
				if step < 1 {
					step = 1
				}
				p.cursor -= step
				if p.cursor < 0 {
					p.cursor = 0
				}
				p.offset = clampOffset(p.cursor, p.offset, len(p.flatRows), p.visibleCount())
			}
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
			// Confirm header: in Project view we use the group's display name;
			// in Recent view we have no group, so fall back to the session's
			// project (already shown after the dot).
			header := sel.Project
			if p.viewMode == config.PickerViewProject {
				row, _ := p.selectedRow()
				if row.GroupIdx >= 0 && row.GroupIdx < len(p.groups) {
					header = p.groups[row.GroupIdx].DisplayName
				}
			}
			c := NewConfirm(
				"Delete session?",
				fmt.Sprintf("%s · %s\nThis cannot be undone.", header, sel.Project),
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
		case key.Matches(msg, p.keys.View):
			if p.viewMode == config.PickerViewRecent {
				p.viewMode = config.PickerViewProject
			} else {
				p.viewMode = config.PickerViewRecent
			}
			p.cursor = 0
			p.offset = 0
			p.rebuildGroups()
			mode := p.viewMode
			return p, func() tea.Msg { return PickerViewModeChangedMsg{Mode: mode} }
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
	// Rows have variable height (see rowHeight). We sum heights into linesUsed
	// to avoid overflowing the panel body. A row that wouldn't fit is dropped.
	budget := p.bodyLines()

	linesUsed := 0
	for i := p.offset; i < len(p.flatRows); i++ {
		row := p.flatRows[i]
		h := p.rowHeight(row)
		if linesUsed+h > budget {
			break
		}
		if linesUsed > 0 {
			b.WriteByte('\n')
		}
		switch row.Kind {
		case RowGroup:
			b.WriteString(p.renderGroupHeader(row, i == p.cursor, width))
		case RowSession:
			meta, color := p.metaForRow(row)
			selected := i == p.cursor
			if h == 2 {
				b.WriteString(p.renderTwoLineSessionRow(meta, color, selected, width))
			} else {
				b.WriteString(p.renderSessionRow(meta, color, selected, width))
			}
		case RowDivider:
			b.WriteString(p.renderDivider(width))
		}
		linesUsed += h
	}
	return b.String()
}

// metaForRow resolves a session row to its SessionMeta and project gutter
// color, dispatching on view mode.
func (p Picker) metaForRow(row PickerRow) (backend.SessionMeta, lipgloss.TerminalColor) {
	if p.viewMode == config.PickerViewRecent {
		m := p.recentMetas[row.SessionIdx]
		return m, projectColor(m.Project, p.theme)
	}
	g := p.groups[row.GroupIdx]
	return g.Sessions[row.SessionIdx], g.Color
}

// rowHeight returns the line count for a row. RowGroup and RowDivider are
// always 1 line; RowSession is 2 lines in Recent view and inside the
// synthetic LIVE group (Project view), 1 line otherwise.
func (p Picker) rowHeight(row PickerRow) int {
	switch row.Kind {
	case RowGroup, RowDivider:
		return 1
	case RowSession:
		if p.viewMode == config.PickerViewRecent {
			return 2
		}
		// Project view: LIVE group rows are 2-line, others 1-line.
		if row.GroupIdx >= 0 && row.GroupIdx < len(p.groups) &&
			p.groups[row.GroupIdx].Name == liveGroupName {
			return 2
		}
		return 1
	}
	return 1
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

	// LIVE pinned group renders with the bullet glyph + green title; no
	// uppercase remap because the name is already shaped for display.
	var name string
	if g.Name == liveGroupName {
		nameStyle := lipgloss.NewStyle().Foreground(g.Color).Bold(true)
		if !selected {
			nameStyle = nameStyle.Faint(true)
		}
		name = nameStyle.Render("◉ LIVE")
	} else {
		nameStyle := lipgloss.NewStyle().Foreground(g.Color).Bold(true)
		if !selected {
			nameStyle = nameStyle.Faint(true)
		}
		name = nameStyle.Render(strings.ToUpper(truncate(g.DisplayName, 30)))
	}

	count := dimStyle.Render(fmt.Sprintf("%s %s", glyph, countLabel(len(g.Sessions), "session")))
	tokStr := dimStyle.Render(humanizeTokens(int64(g.TotalTokens)) + " tok")

	left := gutter + " " + name
	right := count + "  " + tokStr
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 4 {
		gap = 4
	}
	return left + strings.Repeat(" ", gap) + right
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
	id := idStyle.Render(shortDisplayID(m.Kind, m.ID))

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

// renderTwoLineSessionRow renders a session row as id+meta on line 1 and a
// dim, indented project path on line 2. Used in Recent view and inside the
// pinned LIVE group in Project view.
func (p Picker) renderTwoLineSessionRow(m backend.SessionMeta, projectColor lipgloss.TerminalColor, selected bool, width int) string {
	line1 := p.renderSessionRow(m, projectColor, selected, width)
	// Indent under the id column. Layout: gutter(1) + 3 spaces + glyph(1)
	// + space(1) = 6 cols.
	const indent = "      "
	avail := width - lipgloss.Width(indent)
	if avail < 10 {
		// Too narrow for a useful path line — fall back to single line.
		return line1
	}
	path := homePathDisplay(m.Directory)
	if path == "" {
		path = m.Project
	}
	path = leftTruncate(path, avail)
	pathStyle := lipgloss.NewStyle().Foreground(colSubtext0)
	if !selected {
		pathStyle = pathStyle.Faint(true)
	}
	return line1 + "\n" + indent + pathStyle.Render(path)
}

// renderDivider renders a dim horizontal rule used between the live block
// and the ended block in Recent view.
func (p Picker) renderDivider(width int) string {
	if width < 1 {
		return ""
	}
	line := strings.Repeat("─", width)
	return lipgloss.NewStyle().Foreground(colOverlay1).Faint(true).Render(line)
}

// homePathDisplay replaces the user's home dir prefix with "~". Returns the
// input unchanged if home cannot be resolved or the path has no home prefix.
func homePathDisplay(p string) string {
	if p == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// leftTruncate returns s shortened to width by removing leftmost characters
// and prepending "…". Preserves the rightmost characters (most informative
// part of a path — the basename). If s is already short enough, returns
// it unchanged.
func leftTruncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	keep := width - 1
	return "…" + string(rs[len(rs)-keep:])
}

// shortDisplayID returns a short, prefixed display id for a session. Claude
// sessions use a "sesh_" prefix, OpenCode uses "ses_". The body is the first
// 6 lowercase characters of the source id with dashes stripped (typically
// the leading hex of a UUID). Display only — full SessionMeta.ID is still
// used for all internal lookups, deletes, and search matching.
func shortDisplayID(kind session.SourceKind, id string) string {
	body := strings.ToLower(strings.ReplaceAll(id, "-", ""))
	if len(body) > 6 {
		body = body[:6]
	}
	switch kind {
	case session.SourceOpenCode:
		return "ses_" + body
	default:
		return "sesh_" + body
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
		kbdPill("jk/gG", "nav"),
		kbdPill("^d^u", "page"),
		kbdPill("enter", "open"),
		kbdPill("v", "view"),
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

// visibleCount returns the row count used for cursor-scroll clamping. Rows
// can be 1 or 2 lines (see rowHeight). We compute the actual capacity by
// walking from the current offset summing heights until the body budget is
// exhausted. This stays accurate as the user scrolls through regions of
// mixed 1-line and 2-line rows (Project view with LIVE group expanded).
func (p Picker) visibleCount() int {
	body := p.bodyLines()
	if len(p.flatRows) == 0 {
		return body
	}
	used := 0
	count := 0
	for i := p.offset; i < len(p.flatRows); i++ {
		h := p.rowHeight(p.flatRows[i])
		if used+h > body {
			break
		}
		used += h
		count++
	}
	if count < 1 {
		count = 1
	}
	return count
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

// PickerViewModeChangedMsg signals that the picker's view mode toggled
// and the new value should be persisted to config.
type PickerViewModeChangedMsg struct {
	Mode string
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
