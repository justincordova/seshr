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
// earliest compact boundary in the session (i.e. they are inactive context).
func isPreCompact(sess *parser.Session, top topics.Topic) bool {
	if sess == nil || len(sess.CompactBoundaries) == 0 || len(top.TurnIndices) == 0 {
		return false
	}
	// Find the earliest boundary.
	earliest := sess.CompactBoundaries[0].TurnIndex
	for _, cb := range sess.CompactBoundaries[1:] {
		if cb.TurnIndex < earliest {
			earliest = cb.TurnIndex
		}
	}
	return top.TurnIndices[len(top.TurnIndices)-1] < earliest
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
type Overview struct {
	sess      *parser.Session
	topics    []topics.Topic
	allTopics []topics.Topic
	cursor    int
	offset    int
	expanded  map[int]bool
	stats     bool
	width     int
	height    int
	keys      OverviewKeys
	styles    Styles
	search    SearchBar
}

// NewOverview constructs the screen from a parsed session and its topics.
// Topics are displayed latest-first (by first-turn timestamp descending).
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

func (o Overview) Cursor() int         { return o.cursor }
func (o Overview) Expanded(i int) bool { return o.expanded[i] }
func (o Overview) StatsVisible() bool  { return o.stats }
func (o Overview) Init() tea.Cmd       { return nil }

func (o Overview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height
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
				o.offset = clampOffset(o.cursor, o.offset, len(o.topics), o.topicVisibleCount())
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
				o.offset = clampOffset(o.cursor, o.offset, len(o.topics), o.topicVisibleCount())
			}
			return o, nil
		case key.Matches(msg, o.keys.Down):
			if o.cursor < len(o.topics)-1 {
				o.cursor++
				o.offset = clampOffset(o.cursor, o.offset, len(o.topics), o.topicVisibleCount())
			}
			return o, nil
		case key.Matches(msg, o.keys.Expand):
			o.expanded[o.cursor] = !o.expanded[o.cursor]
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
		case key.Matches(msg, o.keys.Edit):
			return o, func() tea.Msg { return OpenEditorMsg{} }
		}
	}
	return o, nil
}

// topicVisibleCount returns the number of topic cards that fit in the main
// panel body. Shared between cursor-scroll clamping and list rendering.
func (o Overview) topicVisibleCount() int {
	if o.height <= 0 || o.width <= 0 {
		return len(o.topics)
	}
	header := o.renderHeader(o.width)
	statsStrip := o.renderStatsStrip(o.width)
	footer := o.renderFooter(o.width)
	fixedH := lipgloss.Height(header) + lipgloss.Height(statsStrip) + lipgloss.Height(footer)
	mainH := o.height - fixedH
	if mainH < 6 {
		mainH = 6
	}
	bodyH := mainH - 4
	cards := bodyH / 3
	if cards < 1 {
		cards = 1
	}
	if cards > len(o.topics) {
		cards = len(o.topics)
	}
	return cards
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
	o.offset = clampOffset(o.cursor, o.offset, len(o.topics), o.topicVisibleCount())
}

func (o Overview) View() string {
	if o.sess == nil {
		return lipgloss.NewStyle().Width(o.width).Padding(1, 2).Render(
			dimStyle.Render("no session loaded"),
		)
	}

	cw := contentWidth(o.width)

	header := o.renderHeader(cw)
	statsStrip := o.renderStatsStrip(cw)
	searchBar := o.search.View(cw)
	footer := o.renderFooter(cw)

	fixedH := lipgloss.Height(header) + lipgloss.Height(statsStrip) +
		lipgloss.Height(searchBar) + lipgloss.Height(footer)
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
	parts = append(parts, footer)
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
			// renderTopicCard returns two lines with no trailing newline;
			// break to the next line before writing turn previews so the
			// first preview doesn't get appended to the card's meta row.
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
	selected := i == o.cursor
	preCompact := isPreCompact(o.sess, top)

	barStyle := lipgloss.NewStyle().Foreground(colSurface1)
	if selected {
		barStyle = lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	} else if preCompact {
		barStyle = lipgloss.NewStyle().Foreground(colSurface1)
	}
	bar := barStyle.Render("▌")

	// Pad raw label to a fixed rune width *before* applying lipgloss styling —
	// fmt %-Ns counts ANSI escape bytes, which misaligns styled strings.
	rawLabel := padRightRaw(truncate(top.Label, 40), 40)
	var label string
	switch {
	case selected:
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
	hints := joinHints(
		kbd("↑↓/jk", "nav"),
		kbd("enter", "expand"),
		kbd("r", "replay"),
		kbd("e", "edit"),
		kbd("tab", "stats"),
		kbd("/", "search"),
		kbd("esc", "back"),
	)
	hintsW := lipgloss.Width(hints)
	gap := (width - hintsW) / 2
	if gap < 2 {
		gap = 2
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Repeat(" ", gap) + hints)
}

// ReturnToPickerMsg tells the root app to swap back to the session picker.
type ReturnToPickerMsg struct{}

// OpenReplayMsg is emitted when the user presses r on the Topic Overview.
type OpenReplayMsg struct{}

type OpenEditorMsg struct{}

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
		preview := padRightRaw(truncate(firstLine(tn.Content), 60), 60)
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
