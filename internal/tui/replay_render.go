package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
	"github.com/sahilm/fuzzy"
)

var markdownRenderer *glamour.TermRenderer
var markdownRendererWidth int

func catppuccinStyleConfig() ansi.StyleConfig {
	cfg := styles.DarkStyleConfig
	textColor := "#cdd6f4"
	headingColor := "#cba6f7"
	linkColor := "#89b4fa"
	codeColor := "#a6e3a1"
	codeBg := "#313244"
	cfg.Document.Color = &textColor
	cfg.H1.Color = &headingColor
	cfg.H2.Color = &headingColor
	cfg.H3.Color = &headingColor
	cfg.H4.Color = &headingColor
	cfg.H5.Color = &headingColor
	cfg.H6.Color = &headingColor
	cfg.Link.Color = &linkColor
	cfg.Code.Color = &codeColor
	cfg.Code.BackgroundColor = &codeBg
	cfg.CodeBlock.Color = &codeColor
	cfg.CodeBlock.BackgroundColor = &codeBg
	return cfg
}

func getMarkdownRenderer(width int) (*glamour.TermRenderer, error) {
	if markdownRenderer != nil && markdownRendererWidth == width {
		return markdownRenderer, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(catppuccinStyleConfig()),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	markdownRenderer = r
	markdownRendererWidth = width
	return r, nil
}

// RenderRoleBadge returns a styled role badge using the pill chrome primitive.
func RenderRoleBadge(role parser.Role, t Theme) string {
	switch role {
	case parser.RoleUser:
		return pill("USER", t.UserColor, t.Background)
	case parser.RoleAssistant:
		return pill("ASST", t.AssistantColor, t.Background)
	case "tool_use":
		return pill("TOOL", t.ToolUseColor, t.Background)
	case parser.RoleToolResult:
		return pill("RSLT", t.ToolResultColor, t.Background)
	default:
		return pill("????", t.ToolResultColor, t.Background)
	}
}

// RenderTimestampDelta formats the gap between prev and curr as +Xh Ym / +Xm Ys / +Xs.
// Returns "" when prev is zero.
func RenderTimestampDelta(prev, curr time.Time) string {
	if prev.IsZero() {
		return ""
	}
	d := curr.Sub(prev)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("+%ds", int(d.Seconds()))
	case d < time.Hour:
		m := int(d / time.Minute)
		s := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("+%dm %ds", m, s)
	default:
		h := int(d / time.Hour)
		m := int((d % time.Hour) / time.Minute)
		return fmt.Sprintf("+%dh %dm", h, m)
	}
}

// RenderTurnHeader renders the one-line header: badge left, delta+tokens right.
func RenderTurnHeader(turn parser.Turn, prev time.Time, width int, s Styles, th Theme) string {
	badge := RenderRoleBadge(turn.Role, th)
	delta := RenderTimestampDelta(prev, turn.Timestamp)
	tokens := fmt.Sprintf("~%d tok", turn.Tokens)

	left := badge
	right := s.Hint.Render(strings.TrimSpace(delta + "  " + tokens))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// RenderMarkdownBody renders markdown to ANSI using glamour's dark style.
// Returns plain text fallback on error.
func RenderMarkdownBody(md string, width int) (string, error) {
	if strings.TrimSpace(md) == "" {
		return "", nil
	}
	r, err := getMarkdownRenderer(width)
	if err != nil {
		return md, nil
	}
	out, err := r.Render(md)
	if err != nil {
		return md, nil
	}
	return out, nil
}

const toolResultPreviewLines = 20

// RenderToolCall renders a tool call as a panel with pretty-printed JSON input.
func RenderToolCall(tc parser.ToolCall, width int, s Styles) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, tc.Input, "", "  "); err != nil {
		pretty.Reset()
		pretty.Write(tc.Input)
	}
	body := wrapBody(sanitize(pretty.String()), panelContentWidth(width))
	return panel(fmt.Sprintf("tool: %s", tc.Name), body, width, 0)
}

// RenderAgentToolCall renders an Agent (subagent) tool call with a distinct
// badge and description extracted from the input JSON.
func RenderAgentToolCall(tc parser.ToolCall, width int, th Theme) string {
	var input struct {
		Description     string `json:"description"`
		SubagentType    string `json:"subagent_type"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(tc.Input, &input); err != nil {
		return RenderToolCall(tc, width, Styles{})
	}

	badge := pill("AGENT", th.AgentColor, th.Background)
	label := input.Description
	if label == "" {
		label = input.SubagentType
	}
	if label == "" {
		label = "subagent"
	}
	tag := ""
	if input.RunInBackground {
		tag = dimStyle.Render(" (background)")
	}
	return badge + " " + label + tag
}

// RenderToolResult renders tool result content, truncated to 20 lines.
func RenderToolResult(result string, isError bool, width int, s Styles) string {
	result = sanitize(result)
	lines := strings.Split(result, "\n")
	truncated := false
	hidden := 0
	if len(lines) > toolResultPreviewLines {
		hidden = len(lines) - toolResultPreviewLines
		lines = lines[:toolResultPreviewLines]
		truncated = true
	}
	body := strings.Join(lines, "\n")
	if truncated {
		body += fmt.Sprintf("\n\n(+%d more lines — enter to expand)", hidden)
	}
	body = wrapBody(body, panelContentWidth(width))
	title := "result"
	if isError {
		title = "error"
	}
	return panel(title, body, width, 0)
}

const panelBorderOverhead = 4

func panelContentWidth(panelWidth int) int {
	w := panelWidth - panelBorderOverhead
	if w < 4 {
		w = 4
	}
	return w
}

// wrapBody wraps each line in body to at most contentW visible characters.
func wrapBody(body string, contentW int) string {
	var b strings.Builder
	first := true
	for _, line := range strings.Split(body, "\n") {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		if lipgloss.Width(line) <= contentW {
			b.WriteString(line)
			continue
		}
		b.WriteString(wrapLine(line, contentW))
	}
	return b.String()
}

// wrapLine hard-wraps a line at contentW visible-width boundaries,
// skipping ANSI escape sequences.
func wrapLine(line string, contentW int) string {
	var b strings.Builder
	col := 0
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			end := strings.IndexByte(line[i:], 'm')
			if end == -1 {
				end = len(line) - i - 1
			}
			b.WriteString(line[i : i+end+1])
			i += end + 1
			continue
		}
		if col >= contentW {
			b.WriteByte('\n')
			col = 0
		}
		_, sz := utf8.DecodeRuneInString(line[i:])
		b.WriteString(line[i : i+sz])
		col++
		i += sz
	}
	return b.String()
}

// RenderSidebar renders the topic list column for Replay mode.
// active is the index of the currently-playing topic; -1 for none.
func RenderSidebar(ts []topics.Topic, active, width int, th Theme) string {
	if width < 4 {
		width = 4
	}
	var b strings.Builder
	for i, t := range ts {
		label := fmt.Sprintf("%d. %s", i+1, t.Label)
		if i == active {
			b.WriteString(truncate("▸ "+label+" ◂", width))
		} else {
			b.WriteString(dimStyle.Render(truncate("  "+label, width)))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// SearchResultsOutput holds the rendered string and the line offset of the
// selected result, used to scroll the viewport to keep it visible.
type SearchResultsOutput struct {
	Content      string
	SelectedLine int // 0-based line number of the selected result header
}

// RenderSearchResults renders all search matches in the main panel.
// Each match shows a header line (badge + turn number) and up to 3 lines of
// excerpt. The selected match is highlighted; others are dimmed.
// Matches with no displayable content are skipped.
func RenderSearchResults(sess *parser.Session, matches []fuzzy.Match, selectedIdx int, width int, s Styles, th Theme) SearchResultsOutput {
	if len(matches) == 0 {
		return SearchResultsOutput{Content: dimStyle.Render("No matches.")}
	}
	const excerptLines = 3
	divider := strings.Repeat("─", width)
	var b strings.Builder
	rendered := 0
	currentLine := 0
	selectedLine := 0
	for i, m := range matches {
		if m.Index < 0 || m.Index >= len(sess.Turns) {
			continue
		}
		turn := sess.Turns[m.Index]
		excerpt := buildExcerpt(turn.Content, excerptLines, width)
		if excerpt == "" {
			excerpt = buildToolExcerpt(turn, width)
		}
		if excerpt == "" {
			continue
		}
		if rendered > 0 {
			b.WriteString(dimStyle.Render(divider))
			b.WriteByte('\n')
			currentLine++
		}
		if i == selectedIdx {
			selectedLine = currentLine
		}
		badge := RenderRoleBadge(turn.Role, th)
		num := dimStyle.Render(fmt.Sprintf("  turn %d", m.Index+1))
		header := badge + num
		selected := i == selectedIdx
		if selected {
			b.WriteString(header)
			b.WriteByte('\n')
			b.WriteString(excerpt)
		} else {
			b.WriteString(dimStyle.Render(header))
			b.WriteByte('\n')
			b.WriteString(dimStyle.Render(excerpt))
		}
		b.WriteByte('\n')
		// header line + excerpt lines + trailing newline
		currentLine += 1 + strings.Count(excerpt, "\n") + 1
		rendered++
	}
	if rendered == 0 {
		return SearchResultsOutput{Content: dimStyle.Render("No matches.")}
	}
	return SearchResultsOutput{Content: b.String(), SelectedLine: selectedLine}
}

// buildToolExcerpt builds a fallback excerpt from tool call names when a turn
// has no text content.
func buildToolExcerpt(turn parser.Turn, width int) string {
	if len(turn.ToolCalls) == 0 {
		return ""
	}
	names := make([]string, 0, len(turn.ToolCalls))
	for _, tc := range turn.ToolCalls {
		names = append(names, tc.Name)
	}
	return truncate(strings.Join(names, ", "), width)
}

// buildExcerpt returns the first n non-empty lines of content, wrapped to width.
func buildExcerpt(content string, n, width int) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	var kept []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		kept = append(kept, truncate(l, width))
		if len(kept) == n {
			break
		}
	}
	return strings.Join(kept, "\n")
}
