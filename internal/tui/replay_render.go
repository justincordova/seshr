package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
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
	return panel(fmt.Sprintf("tool: %s", tc.Name), pretty.String(), width, 0)
}

// RenderToolResult renders tool result content, truncated to 20 lines.
func RenderToolResult(result string, width int, s Styles) string {
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
	return panel("result", body, width, 0)
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
