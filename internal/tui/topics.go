package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
)

// Overview is the Topic Overview Bubbletea model per SPEC §3.2.
type Overview struct {
	sess     *parser.Session
	topics   []topics.Topic
	cursor   int
	expanded map[int]bool
	stats    bool
	width    int
	height   int
	keys     OverviewKeys
	styles   Styles
}

// NewOverview constructs the screen from a parsed session and its topics.
func NewOverview(sess *parser.Session, tops []topics.Topic) Overview {
	return Overview{
		sess:     sess,
		topics:   tops,
		expanded: map[int]bool{},
		keys:     DefaultOverviewKeys(),
		styles:   NewStyles(CatppuccinMocha()),
	}
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
		switch {
		case key.Matches(msg, o.keys.Quit):
			return o, tea.Quit
		case key.Matches(msg, o.keys.Up):
			if o.cursor > 0 {
				o.cursor--
			}
			return o, nil
		case key.Matches(msg, o.keys.Down):
			if o.cursor < len(o.topics)-1 {
				o.cursor++
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
		case key.Matches(msg, o.keys.Replay):
			return o, nil
		case key.Matches(msg, o.keys.Edit):
			return o, nil
		}
	}
	return o, nil
}

func (o Overview) View() string {
	var b strings.Builder
	title := fmt.Sprintf("%s · %s", o.sess.Source, shortID(o.sess.ID))
	b.WriteString(o.styles.Title.Render(title))
	b.WriteString("\n")
	b.WriteString(o.styles.Hint.Render(sessionHeader(o.sess)))
	b.WriteString("\n\n")

	for i, top := range o.topics {
		renderTopicLine(&b, o.styles, top, i, i == o.cursor, o.sess.TokenCount)
		if o.expanded[i] {
			renderExpanded(&b, o.styles, o.sess, top)
		}
	}

	if o.stats {
		b.WriteString("\n")
		b.WriteString(renderStats(o.styles, o.sess, o.topics))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(o.styles.Hint.Render(joinHints(
		kbd("j/k", "navigate"),
		kbd("enter", "expand"),
		kbd("tab", "stats"),
		kbd("esc", "back"),
		kbd("q", "quit"),
	)))
	b.WriteString("\n")
	return o.styles.App.Render(b.String())
}

// ReturnToPickerMsg tells the root app to swap back to the session picker.
type ReturnToPickerMsg struct{}

func sessionHeader(s *parser.Session) string {
	dur := s.ModifiedAt.Sub(s.CreatedAt).Round(time.Minute)
	return fmt.Sprintf("%d turns · ~%s tokens · %s session",
		len(s.Turns),
		humanize.Comma(int64(s.TokenCount)),
		dur,
	)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// TokenBar renders an 8-cell block-char progress bar: █ filled, ░ empty.
// A zero total returns all-empty rather than dividing by zero.
func TokenBar(tokens, total, width int) string {
	if width <= 0 {
		return ""
	}
	filled := 0
	if total > 0 {
		filled = int(float64(tokens) / float64(total) * float64(width))
		if filled > width {
			filled = width
		}
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func renderTopicLine(b *strings.Builder, st Styles, top topics.Topic, i int, selected bool, sessionTokens int) {
	marker := "  "
	if selected {
		marker = "▸ "
	}
	bar := TokenBar(top.TokenCount, sessionTokens, 8)
	line1 := fmt.Sprintf("%s%d. %-40s  %s  ~%s",
		marker,
		i+1,
		truncate(top.Label, 40),
		bar,
		humanize.Comma(int64(top.TokenCount)),
	)
	durMin := int(top.Duration.Minutes())
	line2 := fmt.Sprintf("     turns %d–%d · %d tool calls · %d min",
		firstTurnIdx(top.TurnIndices)+1,
		lastTurnIdx(top.TurnIndices)+1,
		top.ToolCallCount,
		durMin,
	)
	if selected {
		b.WriteString(st.Title.Render(line1))
	} else {
		b.WriteString(line1)
	}
	b.WriteString("\n")
	b.WriteString(st.Hint.Render(line2))
	b.WriteString("\n\n")
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

func renderExpanded(b *strings.Builder, st Styles, sess *parser.Session, top topics.Topic) {
	for _, ix := range top.TurnIndices {
		tn := sess.Turns[ix]
		badge := roleBadge(tn.Role)
		preview := truncate(firstLine(tn.Content), 60)
		line := fmt.Sprintf("     %s  %-60s  ~%d", badge, preview, tn.Tokens)
		b.WriteString(st.Hint.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func roleBadge(r parser.Role) string {
	switch r {
	case parser.RoleUser:
		return "USER "
	case parser.RoleAssistant:
		return "ASST "
	case parser.RoleToolResult:
		return "TOOL "
	default:
		s := strings.ToUpper(string(r)) + "     "
		return s[:5]
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

const defaultContextWindow = 200_000

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
	pct := 0.0
	if defaultContextWindow > 0 {
		pct = 100.0 * float64(sess.TokenCount) / float64(defaultContextWindow)
	}
	lines := []string{
		"── stats ──",
		fmt.Sprintf("total: ~%s tokens (%.1f%% of %s ctx)",
			humanize.Comma(int64(sess.TokenCount)), pct, humanize.Comma(int64(defaultContextWindow))),
		fmt.Sprintf("user: %d turns / ~%s tok",
			roleCounts[parser.RoleUser], humanize.Comma(int64(roleTokens[parser.RoleUser]))),
		fmt.Sprintf("assistant: %d turns / ~%s tok",
			roleCounts[parser.RoleAssistant], humanize.Comma(int64(roleTokens[parser.RoleAssistant]))),
		fmt.Sprintf("tool calls: %d · tool results: %d", tools, roleCounts[parser.RoleToolResult]),
		fmt.Sprintf("%s · %s session · %d files",
			countLabel(len(tops), "topic"), dur, len(fileSet)),
	}
	return st.Hint.Render(strings.Join(lines, "\n"))
}
