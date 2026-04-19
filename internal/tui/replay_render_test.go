package tui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderRoleBadge_KnownRoles(t *testing.T) {
	th := tui.CatppuccinMocha()

	cases := []struct {
		role  parser.Role
		token string
	}{
		{parser.RoleUser, "USER"},
		{parser.RoleAssistant, "ASST"},
		{"tool_use", "TOOL"},
		{parser.RoleToolResult, "RSLT"},
	}

	for _, tc := range cases {
		got := tui.RenderRoleBadge(tc.role, th)
		assert.Truef(t, strings.Contains(got, tc.token),
			"role %q: expected badge to contain %q, got %q", tc.role, tc.token, got)
	}
}

func TestRenderRoleBadge_UnknownRole(t *testing.T) {
	th := tui.CatppuccinMocha()
	got := tui.RenderRoleBadge("garbage", th)
	assert.Contains(t, got, "????")
}

func TestRenderTimestampDelta(t *testing.T) {
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		prev     time.Time
		curr     time.Time
		contains string
	}{
		{"no previous", time.Time{}, base, ""},
		{"47 seconds", base, base.Add(47 * time.Second), "47s"},
		{"3m 22s", base, base.Add(3*time.Minute + 22*time.Second), "3m"},
		{"2h 3m", base, base.Add(2*time.Hour + 3*time.Minute), "2h"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tui.RenderTimestampDelta(tc.prev, tc.curr)
			if tc.contains == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, tc.contains)
		})
	}
}

func TestRenderTurnHeader_ContainsAllParts(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	th := tui.CatppuccinMocha()
	prev := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	curr := prev.Add(3*time.Minute + 22*time.Second)
	turn := parser.Turn{Role: parser.RoleAssistant, Timestamp: curr, Tokens: 890}

	got := tui.RenderTurnHeader(turn, prev, 80, s, th)

	assert.Contains(t, got, "ASST")
	assert.Contains(t, got, "+3m")
	assert.Contains(t, got, "890")
}

func TestRenderMarkdownBody_ReturnsNonEmpty(t *testing.T) {
	input := "Hello **world**\n\nsome code: `Println`\n"

	got, err := tui.RenderMarkdownBody(input, 60)

	require.NoError(t, err)
	assert.Contains(t, got, "Hello")
	assert.Contains(t, got, "Println")
}

func TestRenderMarkdownBody_EmptyInputReturnsEmpty(t *testing.T) {
	got, err := tui.RenderMarkdownBody("", 60)

	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(got))
}

func TestRenderToolCall_BoxContainsNameAndInput(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	tc := parser.ToolCall{Name: "Bash", Input: []byte(`{"command":"ls -la"}`)}

	got := tui.RenderToolCall(tc, 60, s)

	assert.Contains(t, got, "Bash")
	assert.Contains(t, got, "ls -la")
}

func TestRenderToolResult_ShortResultNotTruncated(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	result := "line1\nline2\nline3"

	got := tui.RenderToolResult(result, false, 60, s)

	assert.Contains(t, got, "line1")
	assert.Contains(t, got, "line3")
	assert.NotContains(t, got, "more lines")
}

func TestRenderToolResult_LongResultTruncated(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i+1)
	}
	result := strings.Join(lines, "\n")

	got := tui.RenderToolResult(result, false, 60, s)

	assert.Contains(t, got, "line1")
	assert.Contains(t, got, "line20")
	assert.NotContains(t, got, "line21")
	assert.Contains(t, got, "more lines")
}

func TestRenderToolResult_ErrorShowsErrorTitle(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	got := tui.RenderToolResult("Exit code 1", true, 60, s)
	assert.Contains(t, got, "error")
}

func TestRenderToolResult_SuccessShowsResultTitle(t *testing.T) {
	s := tui.NewStyles(tui.CatppuccinMocha())
	got := tui.RenderToolResult("ok", false, 60, s)
	assert.Contains(t, got, "result")
	assert.NotContains(t, got, "error")
}

func TestRenderSidebar_ActiveHighlighted(t *testing.T) {
	th := tui.CatppuccinMocha()
	topicsList := []topics.Topic{
		{Label: "Setup"},
		{Label: "Auth"},
		{Label: "House"},
	}

	got := tui.RenderSidebar(nil, topicsList, 1, 20, th)

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[1], "Auth")
	assert.Contains(t, lines[0], "Setup")
	assert.Contains(t, lines[2], "House")
}

func TestRenderAgentToolCall_ShowsDescription(t *testing.T) {
	th := tui.CatppuccinMocha()
	tc := parser.ToolCall{
		Name:  "Agent",
		Input: []byte(`{"description":"Fix replay scrolling","subagent_type":"code-reviewer","prompt":"do stuff"}`),
	}

	got := tui.RenderAgentToolCall(tc, 80, th)

	assert.Contains(t, got, "AGENT")
	assert.Contains(t, got, "Fix replay scrolling")
	assert.NotContains(t, got, "background")
}

func TestRenderAgentToolCall_BackgroundTag(t *testing.T) {
	th := tui.CatppuccinMocha()
	tc := parser.ToolCall{
		Name:  "Agent",
		Input: []byte(`{"description":"Background task","run_in_background":true}`),
	}

	got := tui.RenderAgentToolCall(tc, 80, th)

	assert.Contains(t, got, "AGENT")
	assert.Contains(t, got, "Background task")
	assert.Contains(t, got, "background")
}

func TestRenderAgentToolCall_FallbackToSubagentType(t *testing.T) {
	th := tui.CatppuccinMocha()
	tc := parser.ToolCall{
		Name:  "Agent",
		Input: []byte(`{"subagent_type":"code-reviewer"}`),
	}

	got := tui.RenderAgentToolCall(tc, 80, th)

	assert.Contains(t, got, "AGENT")
	assert.Contains(t, got, "code-reviewer")
}
