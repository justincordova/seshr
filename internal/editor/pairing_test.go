package editor_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/editor"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
	"github.com/stretchr/testify/assert"
)

func TestExpandSelection_WholeTopicSelectsAllTurns(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{
		{Role: parser.RoleUser}, {Role: parser.RoleAssistant},
		{Role: parser.RoleUser}, {Role: parser.RoleAssistant},
	}}
	ts := []topics.Topic{{TurnIndices: []int{0, 1}}, {TurnIndices: []int{2, 3}}}
	sel := editor.Selection{Topics: map[int]bool{1: true}}

	got := editor.ExpandSelection(sess, ts, sel)

	assert.True(t, got.Turns[2])
	assert.True(t, got.Turns[3])
	assert.False(t, got.Turns[0])
}

func TestExpandSelection_UserTurnPullsInAssistant(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{
		{Role: parser.RoleUser}, {Role: parser.RoleAssistant},
	}}
	ts := []topics.Topic{{TurnIndices: []int{0, 1}}}
	sel := editor.Selection{Turns: map[int]bool{0: true}}

	got := editor.ExpandSelection(sess, ts, sel)

	assert.True(t, got.Turns[1], "assistant reply must be pulled in with user turn")
}

func TestExpandSelection_ToolUsePullsInToolResult(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{
		{Role: parser.RoleAssistant, ToolCalls: []parser.ToolCall{{ID: "t1"}}},
		{Role: parser.RoleToolResult, ToolResults: []parser.ToolResult{{ID: "t1"}}},
	}}
	ts := []topics.Topic{{TurnIndices: []int{0, 1}}}
	sel := editor.Selection{Turns: map[int]bool{0: true}}

	got := editor.ExpandSelection(sess, ts, sel)

	assert.True(t, got.Turns[1], "tool_result must be pulled in with tool_use")
}

func TestExpandSelection_SystemAndSummaryAreNotSelectable(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{
		{Role: parser.RoleSystem},
		{Role: parser.RoleSummary},
	}}
	ts := []topics.Topic{{TurnIndices: []int{0, 1}}}
	sel := editor.Selection{Turns: map[int]bool{0: true, 1: true}}

	got := editor.ExpandSelection(sess, ts, sel)

	assert.False(t, got.Turns[0])
	assert.False(t, got.Turns[1])
}
