package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
)

// LandingKeys are the keybindings for the Landing page.
type LandingKeys struct {
	Topics     key.Binding
	Replay     key.Binding
	Resume     key.Binding
	Info       key.Binding
	LivePicker key.Binding
	Back       key.Binding
	Search     key.Binding
	Quit       key.Binding
}

// DefaultLandingKeys returns the landing page bindings.
func DefaultLandingKeys() LandingKeys {
	return LandingKeys{
		Topics:     key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "topics")),
		Replay:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "replay")),
		Resume:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "resume")),
		Info:       key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
		LivePicker: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "live picker")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// LandingModel renders the per-session summary shown on enter from the picker.
type LandingModel struct {
	view   *SessionView
	meta   backend.SessionMeta
	keys   LandingKeys
	width  int
	height int
	th     Theme
	styles Styles
}

// NewLandingModel returns a LandingModel for the given session view.
func NewLandingModel(view *SessionView, th Theme) LandingModel {
	return LandingModel{
		view:   view,
		meta:   view.Meta,
		keys:   DefaultLandingKeys(),
		th:     th,
		styles: NewStyles(th),
	}
}

func (m LandingModel) Init() tea.Cmd { return nil }

func (m LandingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(km, m.keys.Back):
			return m, func() tea.Msg { return ReturnToPickerMsg{} }
		case key.Matches(km, m.keys.Topics):
			return m, func() tea.Msg { return OpenOverviewMsg{} }
		case key.Matches(km, m.keys.Replay):
			return m, func() tea.Msg { return OpenReplayMsg{} }
		case key.Matches(km, m.keys.Resume):
			return m, func() tea.Msg { return OpenResumeOverlayMsg{} }
		case key.Matches(km, m.keys.Quit), km.String() == "q":
			return m, tea.Quit
		}
	}
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}
	return m, nil
}

func (m LandingModel) View() string {
	if m.view == nil {
		return ""
	}
	cw := contentWidth(m.width)
	var b strings.Builder

	sess := m.view.Session
	meta := m.meta
	live := m.view.Live

	// ── Header line ──────────────────────────────────────────────────────────
	stateStr := ""
	if live != nil {
		switch live.Status {
		case backend.StatusWorking:
			stateStr = lipgloss.NewStyle().Foreground(m.th.Success).Bold(true).Render("WORKING ●")
		case backend.StatusWaiting:
			stateStr = lipgloss.NewStyle().Foreground(m.th.Warning).Bold(true).Render("WAITING ●")
		default:
			stateStr = lipgloss.NewStyle().Foreground(m.th.Muted).Render("? LIVE")
		}
	} else {
		stateStr = lipgloss.NewStyle().Foreground(m.th.Muted).Render("ended " + humanize.Time(meta.UpdatedAt))
	}
	idStyle := lipgloss.NewStyle().Foreground(m.th.Foreground).Bold(true)
	header := idStyle.Render(truncate(meta.ID, 36)) + " · " +
		dimStyle.Render(meta.Project) + " · " +
		dimStyle.Render(sourceBadge(meta.Kind)) + " · " +
		stateStr
	b.WriteString(lipgloss.NewStyle().Width(cw).Padding(1, 2, 0).Render(header))
	b.WriteString("\n")

	// ── Stats line ───────────────────────────────────────────────────────────
	statsItems := []string{
		fmt.Sprintf("%d turns", len(sess.Turns)),
		humanizeTokens(int64(sess.TokenCount)) + " tok",
	}
	if meta.CostUSD > 0 {
		statsItems = append(statsItems, fmt.Sprintf("$%.2f", meta.CostUSD))
	}
	if len(sess.CompactBoundaries) > 0 {
		statsItems = append(statsItems, fmt.Sprintf("%d compactions", len(sess.CompactBoundaries)))
	}
	if live != nil && live.ContextTokens > 0 && live.ContextWindow > 0 {
		pct := live.ContextTokens * 100 / live.ContextWindow
		if pct >= 80 {
			statsItems = append(statsItems, lipgloss.NewStyle().Foreground(m.th.Warning).Render(fmt.Sprintf("context %d%% ⚠", pct)))
		}
	}
	statsLine := dimStyle.Render(strings.Join(statsItems, " · "))
	b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(statsLine))
	b.WriteString("\n\n")

	// ── Key facts ────────────────────────────────────────────────────────────
	labelStyle := lipgloss.NewStyle().Foreground(m.th.Accent).Width(18)
	if live != nil {
		if live.CurrentTask != "" {
			b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(
				labelStyle.Render("Current action:") + dimStyle.Render(truncate(live.CurrentTask, 60)+" · "+humanize.Time(live.LastActivity)),
			))
			b.WriteString("\n")
		}
	} else {
		firstPrompt := firstUserContent(sess)
		if firstPrompt != "" {
			b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(
				labelStyle.Render("First prompt:") + dimStyle.Render(fmt.Sprintf("%q", truncate(firstPrompt, 60))),
			))
			b.WriteString("\n")
		}
		lastAction := lastToolUseLine(sess)
		if lastAction != "" {
			b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(
				labelStyle.Render("Last action:") + dimStyle.Render(lastAction+" · "+humanize.Time(meta.UpdatedAt)),
			))
			b.WriteString("\n")
		}
	}

	// ── Files touched ────────────────────────────────────────────────────────
	if files := topFiles(m.view.Topics); len(files) > 0 {
		label := "Files touched:"
		if live != nil {
			label = "Files in play:"
		}
		overflow := len(files) - 5
		shown := files
		if len(shown) > 5 {
			shown = shown[:5]
		}
		fileLine := strings.Join(shown, ", ")
		if overflow > 0 {
			fileLine += fmt.Sprintf("  (+%d)", overflow)
		}
		b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(
			labelStyle.Render(label) + dimStyle.Render(fileLine),
		))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// ── Token bar (collapsed) ─────────────────────────────────────────────────
	tokenLine := renderCollapsedTokenBar(sess, cw, m.th)
	b.WriteString(lipgloss.NewStyle().Width(cw).Padding(0, 2).Render(tokenLine))
	b.WriteString("\n\n")

	// ── Footer ───────────────────────────────────────────────────────────────
	footerHints := []string{
		kbdPill("t", "topics"),
		kbdPill("r", "replay"),
		kbdPill("c", "resume"),
		kbdPill("esc", "back"),
	}
	footer := renderCenteredFooter(footerHints, cw)
	b.WriteString(footer)

	return centerBlock(b.String(), m.width)
}

// firstUserContent returns the first user turn's content, or "".
func firstUserContent(sess *session.Session) string {
	for _, t := range sess.Turns {
		if t.Role == session.RoleUser {
			return t.Content
		}
	}
	return ""
}

// lastToolUseLine extracts the most-recent tool use line from the session.
func lastToolUseLine(sess *session.Session) string {
	last := ""
	for _, t := range sess.Turns {
		if t.Role == session.RoleAssistant && len(t.ToolCalls) > 0 {
			tc := t.ToolCalls[len(t.ToolCalls)-1]
			last = tc.Name
		}
	}
	return last
}

// topFiles returns the unique files across all topics, ordered by frequency.
func topFiles(ts []topics.Topic) []string {
	freq := map[string]int{}
	for _, top := range ts {
		for _, f := range top.FileSet {
			freq[f]++
		}
	}
	type kv struct {
		f string
		n int
	}
	var pairs []kv
	for f, n := range freq {
		pairs = append(pairs, kv{f, n})
	}
	// Sort by frequency desc, then alpha.
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].n > pairs[i].n || (pairs[j].n == pairs[i].n && pairs[j].f < pairs[i].f) {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.f
	}
	return out
}

// renderCollapsedTokenBar renders the collapsed token bar for the landing page.
func renderCollapsedTokenBar(sess *session.Session, width int, th Theme) string {
	if sess.TokenCount == 0 {
		return ""
	}
	total := int64(sess.TokenCount)
	barWidth := width - 4
	if barWidth < 10 {
		barWidth = 10
	}

	bar := strings.Repeat("█", barWidth)
	barLine := lipgloss.NewStyle().Foreground(th.Accent).Render(bar)
	totalStr := "~" + humanizeTokens(total) + " total"

	return "Tokens\n" + barLine + " " + dimStyle.Render(totalStr)
}

// OpenOverviewMsg navigates to the Topic Overview.
type OpenOverviewMsg struct{}

// OpenResumeOverlayMsg opens the resume overlay.
type OpenResumeOverlayMsg struct{}
