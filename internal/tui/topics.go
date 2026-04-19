package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/seshr/internal/editor"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
)

// compactDividerAfter returns a set of topic indices after which a compact
// divider should be rendered. For each compact boundary, the divider goes
// after the last topic whose turns all precede the boundary TurnIndex.
func compactDividerAfter(sess *parser.Session, tops []topics.Topic) map[int]parser.CompactBoundary {
	if sess == nil || len(sess.CompactBoundaries) == 0 {
		return nil
	}
	result := make(map[int]parser.CompactBoundary, len(sess.CompactBoundaries))
	for _, cb := range sess.CompactBoundaries {
		// Find the last topic index whose last turn falls before cb.TurnIndex.
		dividerAfter := -1
		for i, top := range tops {
			if len(top.TurnIndices) == 0 {
				continue
			}
			lastTurn := top.TurnIndices[len(top.TurnIndices)-1]
			if lastTurn < cb.TurnIndex {
				dividerAfter = i
			}
		}
		if dividerAfter >= 0 {
			result[dividerAfter] = cb
		}
	}
	return result
}

// isPreCompact returns true when all of a topic's turns fall before the
// last compact boundary in the session. Only topics after the final boundary
// are in the active context; everything before any boundary is inactive.
func isPreCompact(sess *parser.Session, top topics.Topic) bool {
	if sess == nil || len(sess.CompactBoundaries) == 0 || len(top.TurnIndices) == 0 {
		return false
	}
	// Find the latest boundary — everything before it is out of active context.
	latest := sess.CompactBoundaries[0].TurnIndex
	for _, cb := range sess.CompactBoundaries[1:] {
		if cb.TurnIndex > latest {
			latest = cb.TurnIndex
		}
	}
	return top.TurnIndices[len(top.TurnIndices)-1] < latest
}

// renderCompactDivider renders the accent-colored rule shown between pre-compact
// and post-compact topics.
func renderCompactDivider(cb parser.CompactBoundary, width int) string {
	var meta string
	if cb.Trigger != "" {
		meta = cb.Trigger
		if cb.PreTokens > 0 {
			meta += fmt.Sprintf(" · %s tok", humanize.Comma(int64(cb.PreTokens)))
		}
		if cb.DurationMs > 0 {
			dur := time.Duration(cb.DurationMs) * time.Millisecond
			meta += fmt.Sprintf(" · %s", dur.Round(time.Second))
		}
	}
	label := "── compacted"
	if meta != "" {
		label += " ─ " + meta
	}
	// Pad with dashes to fill width.
	labelRunes := []rune(label)
	remaining := width - len(labelRunes)
	if remaining > 0 {
		label += " " + strings.Repeat("─", remaining-1)
	}
	return lipgloss.NewStyle().Foreground(colBlue).Render(truncate(label, width))
}

// Overview is the Topic Overview Bubbletea model per SPEC §3.2.
// It also hosts selection and pruning (previously the separate Editor screen).
type Overview struct {
	sess      *parser.Session
	topics    []topics.Topic
	allTopics []topics.Topic
	cursor    int
	offset    int
	expanded  map[int]bool
	selected  map[int]bool
	stats     bool
	pruning   bool
	status    string
	confirm   *Confirm
	width     int
	height    int
	keys      OverviewKeys
	styles    Styles
	search    SearchBar
}

// NewOverview constructs the screen from a parsed session and its topics.
// Topics are displayed in chronological order (oldest first).
func NewOverview(sess *parser.Session, tops []topics.Topic) Overview {
	sorted := make([]topics.Topic, len(tops))
	copy(sorted, tops)
	sort.SliceStable(sorted, func(i, j int) bool {
		return topicStartTime(sess, sorted[i]).Before(topicStartTime(sess, sorted[j]))
	})
	return Overview{
		sess:      sess,
		topics:    sorted,
		allTopics: sorted,
		expanded:  map[int]bool{},
		selected:  map[int]bool{},
		keys:      DefaultOverviewKeys(),
		styles:    NewStyles(CatppuccinMocha()),
		search:    NewSearchBar(),
	}
}

func topicStartTime(sess *parser.Session, top topics.Topic) time.Time {
	if sess == nil || len(top.TurnIndices) == 0 {
		return time.Time{}
	}
	ix := top.TurnIndices[0]
	if ix < 0 || ix >= len(sess.Turns) {
		return time.Time{}
	}
	return sess.Turns[ix].Timestamp
}

func (o Overview) Cursor() int           { return o.cursor }
func (o Overview) Offset() int           { return o.offset }
func (o Overview) Expanded(i int) bool   { return o.expanded[i] }
func (o Overview) StatsVisible() bool    { return o.stats }
func (o Overview) IsSelected(i int) bool { return o.selected[i] }
func (o Overview) Pruning() bool         { return o.pruning }
func (o Overview) Init() tea.Cmd         { return nil }

func (o Overview) selectedCount() int {
	n := 0
	for _, v := range o.selected {
		if v {
			n++
		}
	}
	return n
}

func (o Overview) tokensFreed() int {
	sum := 0
	for i, sel := range o.selected {
		if sel && i >= 0 && i < len(o.topics) {
			sum += o.topics[i].TokenCount
		}
	}
	return sum
}

// selectionContext classifies selected topics as pre-compact or active.
func (o Overview) selectionContext() (preCount, preTok, activeCount, activeTok int) {
	for i, sel := range o.selected {
		if !sel || i < 0 || i >= len(o.topics) {
			continue
		}
		top := o.topics[i]
		if isPreCompact(o.sess, top) {
			preCount++
			preTok += top.TokenCount
		} else {
			activeCount++
			activeTok += top.TokenCount
		}
	}
	return
}

func (o Overview) currentSelection() editor.Selection {
	sel := editor.Selection{Topics: map[int]bool{}}
	for i, v := range o.selected {
		if v {
			sel.Topics[i] = true
		}
	}
	return editor.ExpandSelection(o.sess, o.topics, sel)
}

func (o Overview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Confirmation dialog intercepts all input when active.
	if o.confirm != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			cm, _ := o.confirm.Update(km)
			c := cm.(Confirm)
			o.confirm = &c
			if c.Done() {
				o.confirm = nil
				if c.Confirmed() {
					sel := o.currentSelection()
					o.pruning = true
					o.status = "pruning…"
					return o, pruneCmd(o.sess, sel)
				}
			}
			return o, nil
		}
		return o, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height
		o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
		return o, nil
	case PruneDoneMsg:
		o.pruning = false
		o.status = fmt.Sprintf("pruned %d turns", msg.RemovedTurns)
		o.selected = map[int]bool{}
		return o, tea.Batch(LoadSessionCmd(o.sess.Path), clearStatusCmd())
	case PruneErrMsg:
		o.pruning = false
		o.status = msg.Err.Error()
		return o, nil
	case SessionLoadedMsg:
		o.sess = msg.Session
		tops := make([]topics.Topic, len(msg.Topics))
		copy(tops, msg.Topics)
		sort.SliceStable(tops, func(i, j int) bool {
			return topicStartTime(o.sess, tops[i]).Before(topicStartTime(o.sess, tops[j]))
		})
		o.topics = tops
		o.allTopics = tops
		o.cursor = 0
		o.offset = 0
		o.expanded = map[int]bool{}
		return o, nil
	case clearStatusMsg:
		o.status = ""
		return o, nil
	case tea.KeyMsg:
		if o.search.Active() {
			switch msg.String() {
			case "esc":
				o.search.Close()
				o.topics = o.allTopics
				o.expanded = map[int]bool{}
				if o.cursor >= len(o.topics) {
					o.cursor = len(o.topics) - 1
				}
				if o.cursor < 0 {
					o.cursor = 0
				}
				o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
				return o, nil
			case "enter":
				o.search.Commit()
				o.applyTopicSearchFilter()
				return o, nil
			case "up", "ctrl+p":
				if o.cursor > 0 {
					o.cursor--
				}
				return o, nil
			case "down", "ctrl+n":
				if o.cursor < len(o.topics)-1 {
					o.cursor++
				}
				return o, nil
			default:
				o.search.Update(msg)
				o.applyTopicSearchFilter()
				return o, nil
			}
		}
		switch {
		case key.Matches(msg, o.keys.Quit):
			return o, tea.Quit
		case key.Matches(msg, o.keys.Up):
			if o.cursor > 0 {
				o.cursor--
				o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
			}
			return o, nil
		case key.Matches(msg, o.keys.Down):
			if o.cursor < len(o.topics)-1 {
				o.cursor++
				o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
			}
			return o, nil
		case key.Matches(msg, o.keys.Expand):
			o.expanded[o.cursor] = !o.expanded[o.cursor]
			o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
			return o, nil
		case key.Matches(msg, o.keys.FoldAll):
			// If any are expanded, collapse all. Otherwise expand all.
			anyExpanded := false
			for _, v := range o.expanded {
				if v {
					anyExpanded = true
					break
				}
			}
			if anyExpanded {
				o.expanded = map[int]bool{}
			} else {
				for i := range o.topics {
					o.expanded[i] = true
				}
			}
			o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
			return o, nil
		case key.Matches(msg, o.keys.Select):
			o.selected[o.cursor] = !o.selected[o.cursor]
			return o, nil
		case key.Matches(msg, o.keys.ToggleAll):
			// If any are unselected, select all. Otherwise deselect all.
			allSelected := len(o.selected) == len(o.topics)
			if allSelected {
				for _, v := range o.selected {
					if !v {
						allSelected = false
						break
					}
				}
			}
			if allSelected && len(o.topics) > 0 {
				o.selected = map[int]bool{}
			} else {
				for i := range o.topics {
					o.selected[i] = true
				}
			}
			return o, nil
		case key.Matches(msg, o.keys.Prune):
			if o.pruning || o.selectedCount() == 0 {
				return o, nil
			}
			sel := o.currentSelection()
			preCount, preTok, activeCount, activeTok := o.selectionContext()
			var confirmTitle, confirmBody string
			if activeCount == 0 {
				confirmTitle = fmt.Sprintf("Prune %d pre-compact topics?", preCount)
				confirmBody = fmt.Sprintf("Turns removed: %d (~%s tokens)\n✓ These are not in the active context and can be safely removed.\nA .bak backup will be created automatically.",
					len(sel.Turns), humanize.Comma(int64(preTok)))
			} else {
				confirmTitle = fmt.Sprintf("Prune %d topics?", o.selectedCount())
				confirmBody = fmt.Sprintf("Turns removed: %d (~%s tokens)\n⚠ ~%s of these tokens are in the active context window.\nType /clear in Claude Code before resuming this session.\nA .bak backup will be created automatically.",
					len(sel.Turns),
					humanize.Comma(int64(o.tokensFreed())),
					humanize.Comma(int64(activeTok)))
			}
			o.confirm = &Confirm{
				title:  confirmTitle,
				body:   confirmBody,
				styles: o.styles,
			}
			return o, nil
		case key.Matches(msg, o.keys.Stats):
			o.stats = !o.stats
			return o, nil
		case key.Matches(msg, o.keys.Back):
			return o, func() tea.Msg { return ReturnToPickerMsg{} }
		case key.Matches(msg, o.keys.Search):
			o.search.Open()
			return o, nil
		case key.Matches(msg, o.keys.Replay):
			return o, func() tea.Msg { return OpenReplayMsg{} }
		}
	}
	return o, nil
}

// heightOrZero returns lipgloss.Height of s, or 0 when s is empty.
// lipgloss.Height("") returns 1 which over-counts the space taken by
// conditionally-rendered components like the search bar.
func heightOrZero(s string) int {
	if s == "" {
		return 0
	}
	return lipgloss.Height(s)
}

// topicBodyHeight returns the line budget available to the topic list body,
// i.e. the area inside the panel border (so excludes header, stats strip,
// search bar, selection strip, footer, panel border, and panel title).
func (o Overview) topicBodyHeight() int {
	if o.height <= 0 || o.width <= 0 {
		return 20
	}
	cw := contentWidth(o.width)
	header := o.renderHeader(cw)
	statsStrip := o.renderStatsStrip(cw)
	footer := o.renderFooter(cw)
	selStrip := o.renderSelectionStrip(cw)
	searchBar := o.search.View(cw)
	fixedH := heightOrZero(header) + heightOrZero(statsStrip) +
		heightOrZero(footer) + heightOrZero(selStrip) + heightOrZero(searchBar)
	mainH := o.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	// Panel border (2) + title row (1) + spacing = 5 lines outside body.
	// This matches renderTopicPanel: bodyH := height - 5.
	bodyH := mainH - 5
	if bodyH < 1 {
		bodyH = 1
	}
	return bodyH
}

// cursorVisibleFrom reports whether the cursor topic would be fully rendered
// (2-line card + any expanded previews + a trailing blank) when the list is
// drawn starting at fromIdx with bodyH available lines. This mirrors the
// budget logic in renderTopicList exactly so scroll math tracks render math.
func (o Overview) cursorVisibleFrom(fromIdx, cursor, bodyH int) bool {
	if cursor < fromIdx || cursor >= len(o.topics) {
		return false
	}
	dividerAfter := compactDividerAfter(o.sess, o.topics)
	linesUsed := 0
	for i := fromIdx; i <= cursor; i++ {
		need := 2
		if linesUsed > 0 {
			need++ // separator above the card
		}
		if linesUsed+need > bodyH {
			return false
		}
		linesUsed += need
		if o.expanded[i] {
			linesUsed++ // blank line above expansion block
			extra := len(o.topics[i].TurnIndices) + 1
			if extra > maxExpandedPreviews+1 {
				extra = maxExpandedPreviews + 1
			}
			// For the cursor itself, require the whole expansion to fit so
			// its turns are actually visible. For earlier topics an over-fill
			// just pushes the cursor off-screen, which is the same failure.
			if linesUsed+extra > bodyH {
				return false
			}
			linesUsed += extra
		}
		if _, ok := dividerAfter[i]; ok {
			if i < cursor {
				linesUsed += 2
			}
		}
	}
	return true
}

// clampTopicOffset returns the smallest offset in [0, cursor] such that the
// cursor topic (and its expansion, if any) is fully visible when rendering
// starts at that offset. Handles variable-height topics (expanded cards +
// compact dividers) by walking the actual render budget, not a fixed window.
func (o Overview) clampTopicOffset(cursor, offset, bodyH int) int {
	if len(o.topics) == 0 || bodyH <= 0 {
		return 0
	}
	if cursor < 0 {
		return 0
	}
	// If cursor moved above the current scroll window, snap offset to cursor.
	if cursor < offset {
		return cursor
	}
	// Advance offset until cursor fits in the render budget. Bounded by
	// cursor: at worst offset == cursor shows the cursor card first, with
	// whatever expansion budget remains.
	for offset < cursor && !o.cursorVisibleFrom(offset, cursor, bodyH) {
		offset++
	}
	return offset
}

func (o *Overview) applyTopicSearchFilter() {
	if o.search.Query() == "" {
		o.topics = o.allTopics
	} else {
		haystack := make([]string, len(o.allTopics))
		for i, t := range o.allTopics {
			var b strings.Builder
			b.WriteString(t.Label)
			b.WriteString(" ")
			for _, idx := range t.TurnIndices {
				if idx >= 0 && idx < len(o.sess.Turns) {
					b.WriteString(o.sess.Turns[idx].Content)
					b.WriteString(" ")
				}
			}
			haystack[i] = b.String()
		}
		o.search.Filter(haystack)
		o.topics = make([]topics.Topic, 0, len(o.search.Matches()))
		for _, m := range o.search.Matches() {
			o.topics = append(o.topics, o.allTopics[m.Index])
		}
	}
	o.expanded = map[int]bool{}
	if o.cursor >= len(o.topics) {
		o.cursor = len(o.topics) - 1
	}
	if o.cursor < 0 {
		o.cursor = 0
	}
	o.offset = o.clampTopicOffset(o.cursor, o.offset, o.topicBodyHeight())
}

func (o Overview) View() string {
	if o.confirm != nil {
		return o.confirm.View()
	}

	if o.sess == nil {
		return lipgloss.NewStyle().Width(o.width).Padding(1, 2).Render(
			dimStyle.Render("no session loaded"),
		)
	}

	cw := contentWidth(o.width)

	header := o.renderHeader(cw)
	statsStrip := o.renderStatsStrip(cw)
	searchBar := o.search.View(cw)
	selStrip := o.renderSelectionStrip(cw)
	footer := o.renderFooter(cw)

	fixedH := heightOrZero(header) + heightOrZero(statsStrip) +
		heightOrZero(searchBar) + heightOrZero(selStrip) + heightOrZero(footer)
	mainH := o.height - fixedH
	if o.height == 0 || mainH < 6 {
		mainH = len(o.topics)*3 + 20
	}
	var main string
	if o.stats {
		main = o.renderStatsPanel(cw, mainH)
	} else {
		main = o.renderTopicPanel(cw, mainH)
	}

	parts := []string{header, statsStrip, main}
	if searchBar != "" {
		parts = append(parts, searchBar)
	}
	parts = append(parts, selStrip, footer)
	return centerBlock(lipgloss.JoinVertical(lipgloss.Left, parts...), o.width)
}

func (o Overview) renderStatsPanel(width, height int) string {
	title := fmt.Sprintf(" Stats %s", dimStyle.Render("(tab to return)"))
	body := renderStats(o.styles, o.sess, o.topics)
	return panel(title, body, width, height)
}

func (o Overview) renderHeader(width int) string {
	logo := renderLogo()
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(logo)
}

func (o Overview) renderStatsStrip(width int) string {
	s := o.sess

	dur := s.ModifiedAt.Sub(s.CreatedAt).Round(time.Minute)
	if dur < 0 {
		dur = -dur
	}
	if dur == 0 {
		dur = s.ModifiedAt.Sub(s.CreatedAt).Round(time.Second)
		if dur < 0 {
			dur = -dur
		}
		if dur == 0 {
			dur = time.Second
		}
	}

	items := []string{
		statInline("TURNS", fmt.Sprintf("%d", len(s.Turns)), colGreen),
		statInline("TOKENS", fmt.Sprintf("~%s", humanize.Comma(int64(s.TokenCount))), colGreen),
		statInline("TOPICS", fmt.Sprintf("%d", len(o.topics)), colMauve),
		statInline("DURATION", dur.String(), colLavender),
	}

	sep := dimStyle.Render("  │  ")
	row := strings.Join(items, sep)
	return lipgloss.NewStyle().Width(width).Padding(0, 2).Render(row)
}

// renderSelectionStrip renders the 2-line selection summary at the bottom.
func (o Overview) renderSelectionStrip(width int) string {
	total := o.selectedCount()
	preCount, preTok, activeCount, activeTok := o.selectionContext()

	var line1, line2 string
	switch {
	case total == 0:
		line1 = dimStyle.Render("no topics selected · space to select")
		line2 = ""
	case preCount > 0 && activeCount > 0:
		line1 = dimStyle.Render(fmt.Sprintf("%d topics selected · ~%s tokens freed (~%s pre-compact, ~%s active)",
			total,
			humanize.Comma(int64(o.tokensFreed())),
			humanize.Comma(int64(preTok)),
			humanize.Comma(int64(activeTok)),
		))
		line2 = o.styles.Error.Render("⚠ Includes active context turns — requires /clear before resume")
	case activeCount == 0:
		noun := "topics"
		if total == 1 {
			noun = "topic"
		}
		line1 = dimStyle.Render(fmt.Sprintf("%d %s selected · ~%s tokens freed",
			total, noun, humanize.Comma(int64(o.tokensFreed()))))
		line2 = successStyle.Render("✓ Safe to prune — not in active context")
	default:
		noun := "topics"
		if total == 1 {
			noun = "topic"
		}
		line1 = dimStyle.Render(fmt.Sprintf("%d %s selected · ~%s tokens freed",
			total, noun, humanize.Comma(int64(o.tokensFreed()))))
		line2 = o.styles.Error.Render("⚠ Warning: these turns are in the active context")
	}

	if o.status != "" {
		var statusLine string
		if o.pruning || strings.HasPrefix(o.status, "pruned") {
			statusLine = successStyle.Render(o.status)
		} else {
			statusLine = o.styles.Error.Render(o.status)
		}
		line2 = statusLine
	}

	var out string
	if line2 != "" {
		out = line1 + "\n" + line2
	} else {
		out = line1
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 2).Render(out)
}

func (o Overview) renderTopicPanel(width, height int) string {
	title := fmt.Sprintf(" %s %s %s",
		lipgloss.NewStyle().Foreground(colText).Bold(true).Render(string(o.sess.Source)),
		dimStyle.Render("·"),
		dimStyle.Render(shortID(o.sess.ID)),
	)
	bodyH := height - 5
	if bodyH < 2 {
		bodyH = len(o.topics)*3 + 10
	}
	body := o.renderTopicList(width-4, bodyH)
	return panel(title, body, width, height)
}

func (o Overview) renderTopicList(width, bodyH int) string {
	var b strings.Builder
	linesUsed := 0

	dividerAfter := compactDividerAfter(o.sess, o.topics)

	for i := o.offset; i < len(o.topics); i++ {
		top := o.topics[i]

		// Each card always takes 2 lines; need 1 more for separator after first.
		needed := 2
		if linesUsed > 0 {
			needed++
		}
		if linesUsed+needed > bodyH {
			break
		}

		if linesUsed > 0 {
			b.WriteByte('\n')
			linesUsed++
		}
		b.WriteString(o.renderTopicCard(i, top, width))
		linesUsed += 2

		if o.expanded[i] {
			b.WriteByte('\n')
			linesUsed++
			remaining := bodyH - linesUsed
			written := renderExpandedCapped(&b, o.styles, o.sess, top, remaining)
			linesUsed += written
		}

		// Insert compact divider after this topic if a boundary falls here.
		if cb, ok := dividerAfter[i]; ok && linesUsed < bodyH {
			b.WriteByte('\n')
			b.WriteString(renderCompactDivider(cb, width))
			linesUsed += 2
		}
	}
	return b.String()
}

func (o Overview) renderTopicCard(i int, top topics.Topic, width int) string {
	isCursor := i == o.cursor
	isSelected := o.selected[i]
	preCompact := isPreCompact(o.sess, top)

	// Gutter bar: mauve when cursor OR selected; surface1 otherwise.
	var barStyle lipgloss.Style
	switch {
	case isCursor:
		barStyle = lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	case isSelected:
		barStyle = lipgloss.NewStyle().Foreground(colMauve)
	default:
		barStyle = lipgloss.NewStyle().Foreground(colSurface1)
	}
	bar := barStyle.Render("▌")

	// Pad raw label to a fixed rune width *before* applying lipgloss styling —
	// fmt %-Ns counts ANSI escape bytes, which misaligns styled strings.
	rawLabel := padRightRaw(truncate(top.Label, 40), 40)
	var label string
	switch {
	case isCursor:
		label = lipgloss.NewStyle().Foreground(colText).Bold(true).Render(rawLabel)
	case preCompact:
		label = dimStyle.Render(rawLabel)
	default:
		label = textStyle.Render(rawLabel)
	}

	numStyle := dimStyle
	tokenStyle := dimStyle
	if preCompact {
		numStyle = lipgloss.NewStyle().Foreground(colSurface1)
		tokenStyle = lipgloss.NewStyle().Foreground(colSurface1)
	}

	num := numStyle.Render(fmt.Sprintf("%2d.", i+1))
	tokStr := fmt.Sprintf("~%s tok", humanize.Comma(int64(top.TokenCount)))
	// Pre-compact topics get an ░ right-margin indicator.
	inactiveMarker := ""
	if preCompact {
		inactiveMarker = " " + lipgloss.NewStyle().Foreground(colSurface1).Render("░")
	}
	tokens := tokenStyle.Render(tokStr) + inactiveMarker

	left := fmt.Sprintf("%s %s %s", bar, num, label)
	gap := width - lipgloss.Width(left) - lipgloss.Width(tokens)
	if gap < 1 {
		gap = 1
	}
	line1 := left + strings.Repeat(" ", gap) + tokens

	turnRange := dimStyle.Render(fmt.Sprintf("turns %d–%d",
		firstTurnIdx(top.TurnIndices)+1,
		lastTurnIdx(top.TurnIndices)+1))
	toolCalls := dimStyle.Render(fmt.Sprintf("%d tool calls", top.ToolCallCount))
	duration := dimStyle.Render(formatTopicDuration(top.Duration))
	parts := []string{turnRange, toolCalls, duration}
	if ts := topicStartTime(o.sess, top); !ts.IsZero() {
		parts = append(parts, dimStyle.Render(humanize.Time(ts)))
	}
	line2 := "       " + strings.Join(parts, dimStyle.Render(" · "))

	return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
}

// formatTopicDuration renders a compact human duration, falling back to
// seconds for sub-minute topics so they don't all read "0 min".
func formatTopicDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	if d < time.Minute {
		secs := int(d.Seconds())
		if secs < 1 {
			secs = 1
		}
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%d min", int(d.Minutes()))
}

func (o Overview) renderFooter(width int) string {
	hints := []string{
		kbdPill("↑↓/jk", "nav"),
		kbdPill("enter", "expand"),
		kbdPill("f", "fold all"),
		kbdPill("space", "select"),
		kbdPill("a", "toggle all"),
		kbdPill("p", "prune"),
		kbdPill("r", "replay"),
		kbdPill("tab", "stats"),
		kbdPill("/", "search"),
		kbdPill("esc", "back"),
	}
	return renderCenteredFooter(hints, width)
}

// ReturnToPickerMsg tells the root app to swap back to the session picker.
type ReturnToPickerMsg struct{}

// OpenReplayMsg is emitted when the user presses r on the Topic Overview.
type OpenReplayMsg struct{}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func firstTurnIdx(ix []int) int {
	if len(ix) == 0 {
		return 0
	}
	return ix[0]
}

func lastTurnIdx(ix []int) int {
	if len(ix) == 0 {
		return 0
	}
	return ix[len(ix)-1]
}

const maxExpandedPreviews = 8

// turnPreviewLine returns a non-blank preview string for any turn type.
// For turns with no text content it falls back to describing tool calls/results.
func turnPreviewLine(tn parser.Turn) string {
	if line := firstLine(tn.Content); line != "" {
		return line
	}
	// No text content — describe what's in the turn instead.
	if len(tn.ToolCalls) > 0 {
		names := make([]string, 0, len(tn.ToolCalls))
		seen := map[string]bool{}
		for _, tc := range tn.ToolCalls {
			if !seen[tc.Name] {
				names = append(names, tc.Name)
				seen[tc.Name] = true
			}
		}
		return fmt.Sprintf("tool call: %s", strings.Join(names, ", "))
	}
	if len(tn.ToolResults) > 0 {
		return "tool result"
	}
	if tn.Thinking != "" {
		return "thinking…"
	}
	return "(empty)"
}

// renderExpandedCapped writes expanded turn previews into b, consuming at most
// maxLines lines. Returns the number of lines written.
func renderExpandedCapped(b *strings.Builder, st Styles, sess *parser.Session, top topics.Topic, maxLines int) int {
	if sess == nil || maxLines <= 0 {
		return 0
	}
	written := 0
	shown := 0
	for _, ix := range top.TurnIndices {
		if ix < 0 || ix >= len(sess.Turns) {
			continue
		}
		// Reserve 1 line for the trailing blank.
		if written >= maxLines-1 {
			break
		}
		if shown >= maxExpandedPreviews {
			more := len(top.TurnIndices) - shown
			b.WriteString(st.Hint.Render(fmt.Sprintf("       … %d more turns", more)))
			b.WriteString("\n")
			written++
			break
		}
		tn := sess.Turns[ix]
		badge := roleBadge(tn.Role)
		preview := padRightRaw(truncate(turnPreviewLine(tn), 60), 60)
		line := fmt.Sprintf("       %s  %s  ~%d", badge, preview, tn.Tokens)
		b.WriteString(st.Hint.Render(line))
		b.WriteString("\n")
		written++
		shown++
	}
	// Trailing blank line to visually separate from next card.
	if written > 0 && written < maxLines {
		b.WriteString("\n")
		written++
	}
	return written
}

func roleBadge(r parser.Role) string {
	switch r {
	case parser.RoleUser:
		return pill("USER", colBase, colGreen)
	case parser.RoleAssistant:
		return pill("ASST", colBase, colBlue)
	case parser.RoleToolResult:
		return pill("TOOL", colBase, colLavender)
	default:
		return pill(strings.ToUpper(string(r)), colBase, colOverlay0)
	}
}

func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}

func renderStats(st Styles, sess *parser.Session, tops []topics.Topic) string {
	roleCounts := map[parser.Role]int{}
	roleTokens := map[parser.Role]int{}
	fileSet := map[string]struct{}{}
	var tools int
	for _, tn := range sess.Turns {
		roleCounts[tn.Role]++
		roleTokens[tn.Role] += tn.Tokens
		tools += len(tn.ToolCalls)
	}
	for _, top := range tops {
		for _, f := range top.FileSet {
			fileSet[f] = struct{}{}
		}
	}
	dur := sess.ModifiedAt.Sub(sess.CreatedAt).Round(time.Minute)
	if dur == 0 {
		dur = sess.ModifiedAt.Sub(sess.CreatedAt).Round(time.Second)
		if dur == 0 {
			dur = time.Second
		}
	}
	lines := []string{
		"── stats ──",
		fmt.Sprintf("total: ~%s tokens",
			humanize.Comma(int64(sess.TokenCount))),
		fmt.Sprintf("user: %d turns / ~%s tok",
			roleCounts[parser.RoleUser], humanize.Comma(int64(roleTokens[parser.RoleUser]))),
		fmt.Sprintf("assistant: %d turns / ~%s tok",
			roleCounts[parser.RoleAssistant], humanize.Comma(int64(roleTokens[parser.RoleAssistant]))),
		fmt.Sprintf("tool calls: %d · tool results: %d", tools, roleCounts[parser.RoleToolResult]),
		fmt.Sprintf("%s · %s session · %d topic files",
			countLabel(len(tops), "topic"), dur, len(fileSet)),
	}
	if n := len(sess.CompactBoundaries); n > 0 {
		last := sess.CompactBoundaries[n-1]
		compLine := fmt.Sprintf("compactions: %d (last: %s", n, last.Trigger)
		if last.PreTokens > 0 {
			compLine += fmt.Sprintf(", %s tok", humanize.Comma(int64(last.PreTokens)))
		}
		compLine += ")"
		lines = append(lines, compLine)
	}
	return st.Hint.Render(strings.Join(lines, "\n"))
}

// pruneCmd runs the prune operation asynchronously.
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
