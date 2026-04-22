package session

import "time"

// Source identifies which agent platform produced a session.
// v1 ships with only SourceClaude.
type Source string

const (
	SourceClaude Source = "Claude Code"
)

// Role labels a turn. Values mirror Claude Code JSONL record types plus
// "thinking" which is extracted from assistant content blocks.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
	RoleSystem     Role = "system"
	RoleSummary    Role = "summary"
)

// CompactBoundary marks where a /compact call occurred. Turns before the
// boundary are inactive context — Claude Code ignores them on resume.
type CompactBoundary struct {
	TurnIndex  int    // index of the first turn AFTER this boundary
	Trigger    string // "manual" or "auto"
	PreTokens  int    // token count before compaction
	DurationMs int    // how long compaction took
}

// Session is a parsed agent conversation. See SPEC.md §6.
type Session struct {
	ID                string
	Path              string
	Source            Source
	CreatedAt         time.Time
	ModifiedAt        time.Time
	TokenCount        int
	ToolResultTokens  int // estimate-based sum of all attached tool-result content; split out of assistant turns for display
	Turns             []Turn
	ChainedFiles      []string          // populated only when continuation chains are reconstructed (Phase 7)
	CompactBoundaries []CompactBoundary // one per /compact call, ordered by position
}

// Turn is a single role-tagged exchange within a session.
type Turn struct {
	Role                  Role
	Timestamp             time.Time
	Content               string     // flattened text (assistant text blocks joined; user raw)
	ToolCalls             []ToolCall // tool_use blocks within an assistant turn
	ToolResults           []ToolResult
	Thinking              string // extended-thinking block text, empty if none
	RawIndex              int    // 0-based index of the originating JSONL line; used by pruner
	ExtraLineIndices      []int  // file line numbers of attached tool_result records
	Tokens                int    // from message.usage when present, else heuristic
	IsCompactContinuation bool   // true for the "This session is being continued..." message
}

// ToolCall is a single tool_use block within an assistant turn.
type ToolCall struct {
	ID    string // tool_use_id — matches ToolResult.ID
	Name  string
	Input []byte // raw JSON of the input object, preserved for display
}

// ToolResult is a tool_result record, keyed by the tool_use_id it answers.
type ToolResult struct {
	ID      string
	Content string
	IsError bool
}
