package parser

import "time"

// Session is a parsed agent conversation.
//
// TODO(phase-2): populate from Claude Code JSONL records per SPEC.md §6.
type Session struct {
	ID        string
	Path      string
	StartedAt time.Time
	Turns     []Turn
}

// Turn is a single role-tagged exchange within a session.
type Turn struct {
	Role      string
	Timestamp time.Time
	Text      string
	ToolCalls []ToolCall
	Tokens    int
	Raw       []byte
}

// ToolCall is a single tool invocation within a turn.
type ToolCall struct {
	ID     string
	Name   string
	Input  []byte
	Result []byte
}
