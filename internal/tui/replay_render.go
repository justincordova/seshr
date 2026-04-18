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
	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/topics"
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

// RenderSearchResults renders all search matches in the main panel.
// Each match shows a header line (badge + turn number) and up to 3 lines of
// excerpt. The selected match is highlighted; others are dimmed.
// Matches with no displayable content are skipped.
// scrollTop is the persisted scroll offset; the returned int is the updated offset.
func RenderSearchResults(sess *parser.Session, matches []fuzzy.Match, selectedIdx int, width, height, scrollTop int, s Styles, th Theme) (string, int) {
	if len(matches) == 0 {
		return dimStyle.Render("No matches."), 0
	}
	const excerptLines = 3

	// Build all renderable blocks first so we know their heights.
	type block struct {
		matchIdx int
		lines    []string // rendered lines for this block
	}
	var blocks []block
	divider := strings.Repeat("─", width)
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
		badge := RenderRoleBadge(turn.Role, th)
		num := dimStyle.Render(fmt.Sprintf("  turn %d", m.Index+1))
		header := badge + num
		selected := i == selectedIdx
		var blk block
		blk.matchIdx = i
		if len(blocks) > 0 {
			blk.lines = append(blk.lines, dimStyle.Render(divider))
		}
		if selected {
			blk.lines = append(blk.lines, header)
			for _, l := range strings.Split(excerpt, "\n") {
				blk.lines = append(blk.lines, l)
			}
		} else {
			blk.lines = append(blk.lines, dimStyle.Render(header))
			for _, l := range strings.Split(excerpt, "\n") {
				blk.lines = append(blk.lines, dimStyle.Render(l))
			}
		}
		blocks = append(blocks, blk)
	}
	if len(blocks) == 0 {
		return dimStyle.Render("No matches."), 0
	}

	// Find which block is selected (by matchIdx).
	selectedBlock := 0
	for i, blk := range blocks {
		if blk.matchIdx == selectedIdx {
			selectedBlock = i
			break
		}
	}

	// Flatten all lines and track where selected block starts.
	type lineEntry struct{ text string }
	var allLines []string
	blockStart := make([]int, len(blocks)) // line index of first line of each block
	cur := 0
	for i, blk := range blocks {
		blockStart[i] = cur
		for _, l := range blk.lines {
			allLines = append(allLines, l)
			cur++
		}
	}

	// Determine the visible window: only scroll when selection leaves the window.
	selStart := blockStart[selectedBlock]
	selEnd := selStart + len(blocks[selectedBlock].lines) - 1
	maxTop := len(allLines) - height
	if maxTop < 0 {
		maxTop = 0
	}
	if scrollTop > maxTop {
		scrollTop = maxTop
	}
	top := scrollTop
	bottom := top + height - 1
	if selEnd > bottom {
		top = selEnd - height + 1
		if top > maxTop {
			top = maxTop
		}
		bottom = top + height - 1
	}
	if selStart < top {
		top = selStart
		if top < 0 {
			top = 0
		}
		bottom = top + height - 1
	}

	var out strings.Builder
	for i, l := range allLines {
		if i < top {
			continue
		}
		if i > bottom {
			break
		}
		out.WriteString(l)
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n"), top
}

// ComputeSearchScrollTop calculates the scroll offset for search results
// without rendering. This is called from Update() on the pointer receiver
// so the result can be persisted on the model.
func ComputeSearchScrollTop(sess *parser.Session, matches []fuzzy.Match, selectedIdx, width, height, scrollTop int) int {
	if len(matches) == 0 || height <= 0 {
		return 0
	}
	const excerptLines = 3

	type blockInfo struct {
		matchIdx int
		height   int
	}
	var blocks []blockInfo
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
		h := 1 + strings.Count(excerpt, "\n") + 1
		if len(blocks) > 0 {
			h++
		}
		blocks = append(blocks, blockInfo{matchIdx: i, height: h})
	}
	if len(blocks) == 0 {
		return 0
	}

	selectedBlock := 0
	for i, blk := range blocks {
		if blk.matchIdx == selectedIdx {
			selectedBlock = i
			break
		}
	}

	totalLines := 0
	blockStart := make([]int, len(blocks))
	cur := 0
	for i, blk := range blocks {
		blockStart[i] = cur
		cur += blk.height
		totalLines = cur
	}

	selStart := blockStart[selectedBlock]
	selEnd := selStart + blocks[selectedBlock].height - 1
	maxTop := totalLines - height
	if maxTop < 0 {
		maxTop = 0
	}
	if scrollTop > maxTop {
		scrollTop = maxTop
	}
	top := scrollTop
	bottom := top + height - 1
	if selEnd > bottom {
		top = selEnd - height + 1
		if top > maxTop {
			top = maxTop
		}
		bottom = top + height - 1
	}
	if selStart < top {
		top = selStart
		if top < 0 {
			top = 0
		}
		bottom = top + height - 1
	}
	return top
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
