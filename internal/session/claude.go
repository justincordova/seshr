package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/justincordova/seshr/internal/tokenizer"
)

// Claude parses Claude Code JSONL session files per SPEC §6.2.
type Claude struct{}

// NewClaude returns a Claude Code JSONL session.
func NewClaude() *Claude { return &Claude{} }

// Kind returns the source kind for this parser.
func (c *Claude) Kind() SourceKind { return SourceClaude }

// Detect returns true for any path ending in .jsonl. Narrow by design —
// Phase 7 will refine when the OpenCode parser lands.
func (c *Claude) Detect(path string) bool {
	return len(path) >= 6 && path[len(path)-6:] == ".jsonl"
}

// Parse streams the JSONL at path into a Session.
func (c *Claude) Parse(ctx context.Context, path string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	sess := &Session{
		Path:       path,
		Source:     SourceClaude,
		ModifiedAt: info.ModTime(),
	}

	scanner := bufio.NewScanner(f)
	// Claude assistant lines can be large (tool inputs/thinking blobs).
	// 10MB per line should cover anything reasonable.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineNum := -1
	for scanner.Scan() {
		lineNum++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}
		// Capture session ID from the first parseable line before Turn
		// filtering (infrastructure records carry sessionId too).
		if sess.ID == "" {
			sess.ID = turnSessionID(b)
		}
		// Detect compact_boundary system records before the normal turn parse
		// path, since parseLine drops all system records.
		if cb, ok := parseCompactBoundary(b, len(sess.Turns)); ok {
			sess.CompactBoundaries = append(sess.CompactBoundaries, cb)
			continue
		}
		turn, ok := parseLine(b, lineNum)
		if !ok {
			continue
		}
		// First timestamp wins as CreatedAt.
		if sess.CreatedAt.IsZero() && !turn.Timestamp.IsZero() {
			sess.CreatedAt = turn.Timestamp
		}
		// tool_result records attach to the matching assistant turn if found.
		if turn.Role == RoleToolResult && attachToolResult(sess, turn) {
			continue
		}
		sess.Turns = append(sess.Turns, turn)
		sess.TokenCount += turn.Tokens
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	slog.Info("parsed session", "path", path, "turns", len(sess.Turns), "tokens", sess.TokenCount)
	return sess, nil
}

// parseCompactBoundary checks whether b is a system record with
// subtype "compact_boundary". If so it returns the boundary and ok=true.
// nextTurnIndex is the length of sess.Turns at the point this line was read,
// which becomes the first-turn-after-boundary index.
func parseCompactBoundary(b []byte, nextTurnIndex int) (CompactBoundary, bool) {
	var rec rawRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return CompactBoundary{}, false
	}
	if rec.Type != "system" || rec.Subtype != "compact_boundary" {
		return CompactBoundary{}, false
	}
	return CompactBoundary{
		TurnIndex:  nextTurnIndex,
		Trigger:    rec.CompactMetadata.Trigger,
		PreTokens:  rec.CompactMetadata.PreTokens,
		DurationMs: rec.CompactMetadata.DurationMs,
	}, true
}

// parseLine converts one JSONL line into a Turn. Returns ok=false when the
// line is malformed, has an unknown type, or is of a type we intentionally
// drop (file-history-snapshot, progress). Logged at warn for unknown types.
func parseLine(b []byte, lineNum int) (Turn, bool) {
	var rec rawRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		slog.Warn("skipping malformed jsonl line", "line", lineNum, "err", err)
		return Turn{}, false
	}
	switch rec.Type {
	case "user":
		t := userTurn(rec, lineNum)
		if t.Role == RoleToolResult {
			return t, len(t.ToolResults) > 0
		}
		if t.Content == "" {
			return Turn{}, false
		}
		return t, true
	case "assistant":
		t := assistantTurn(rec, lineNum)
		if t.Content == "" && len(t.ToolCalls) == 0 && t.Thinking == "" {
			return Turn{}, false
		}
		return t, true
	case "tool_result":
		return toolResultTurn(rec, lineNum), true
	case "system", "summary":
		return Turn{}, false
	case "", "file-history-snapshot", "progress", "hook":
		// Infrastructure records, silently ignored.
		return Turn{}, false
	default:
		slog.Warn("unknown record type", "type", rec.Type, "line", lineNum)
		return Turn{}, false
	}
}

// userTurn builds a Turn from a user JSONL record. Content may be a plain
// string or an array of content blocks. When blocks contain tool_result
// entries (the standard pattern in Claude Code JSONL), those are extracted
// and the turn is returned with RoleToolResult for attachment to the
// originating assistant turn.
func userTurn(rec rawRecord, lineNum int) Turn {
	msg, _ := decodeMessage(rec.Message)
	blocks, _ := decodeBlocks(msg.Content)

	if len(blocks) == 0 {
		content := flattenContent(msg.Content)
		return Turn{
			Role:                  RoleUser,
			Timestamp:             rec.Timestamp,
			Content:               content,
			RawIndex:              lineNum,
			Tokens:                tokenizer.Estimate(content),
			IsCompactContinuation: strings.HasPrefix(content, "This session is being continued"),
		}
	}

	var toolResults []ToolResult
	var textParts []string
	for _, bl := range blocks {
		switch bl.Type {
		case "tool_result":
			content := extractBlockContent(bl.Content)
			toolResults = append(toolResults, ToolResult{
				ID:      bl.ToolUseID,
				Content: content,
				IsError: bl.IsError,
			})
		case "text":
			textParts = append(textParts, bl.Text)
		}
	}

	if len(toolResults) > 0 && len(textParts) == 0 {
		content := toolResults[0].Content
		return Turn{
			Role:        RoleToolResult,
			Timestamp:   rec.Timestamp,
			Content:     content,
			ToolResults: toolResults,
			RawIndex:    lineNum,
			Tokens:      tokenizer.Estimate(content),
		}
	}

	content := strings.Join(textParts, "\n\n")
	return Turn{
		Role:                  RoleUser,
		Timestamp:             rec.Timestamp,
		Content:               content,
		RawIndex:              lineNum,
		Tokens:                tokenizer.Estimate(content),
		IsCompactContinuation: strings.HasPrefix(content, "This session is being continued"),
	}
}

// extractBlockContent decodes the content field of a tool_result block,
// which may be a plain string or an array of text blocks.
func extractBlockContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []rawBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var out string
	for _, bl := range blocks {
		if bl.Type == "text" && bl.Text != "" {
			if out != "" {
				out += "\n\n"
			}
			out += bl.Text
		}
	}
	return out
}

// assistantTurn builds a Turn from an assistant record, extracting text,
// thinking, and tool_use blocks from the content array.
func assistantTurn(rec rawRecord, lineNum int) Turn {
	msg, _ := decodeMessage(rec.Message)
	blocks, _ := decodeBlocks(msg.Content)

	turn := Turn{
		Role:      RoleAssistant,
		Timestamp: rec.Timestamp,
		RawIndex:  lineNum,
	}

	var text string
	if len(blocks) == 0 {
		text = flattenContent(msg.Content)
	}
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if text != "" {
				text += "\n\n"
			}
			text += bl.Text
		case "thinking":
			if turn.Thinking != "" {
				turn.Thinking += "\n\n"
			}
			turn.Thinking += bl.Thinking
		case "tool_use":
			turn.ToolCalls = append(turn.ToolCalls, ToolCall{
				ID:    bl.ID,
				Name:  bl.Name,
				Input: bl.Input,
			})
		}
	}
	turn.Content = text

	usage := tokenizer.Usage{
		InputTokens:              msg.Usage.InputTokens,
		OutputTokens:             msg.Usage.OutputTokens,
		CacheCreationInputTokens: msg.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     msg.Usage.CacheReadInputTokens,
	}
	if u := tokenizer.FromUsage(usage); u > 0 {
		turn.Tokens = u
	} else {
		turn.Tokens = tokenizer.Estimate(text + turn.Thinking)
	}
	return turn
}

// toolResultTurn builds a standalone tool-result turn. If attachToolResult
// finds a matching assistant turn it is merged there instead.
func toolResultTurn(rec rawRecord, lineNum int) Turn {
	msg, _ := decodeMessage(rec.Message)
	content := flattenContent(msg.Content)
	return Turn{
		Role:      RoleToolResult,
		Timestamp: rec.Timestamp,
		Content:   content,
		ToolResults: []ToolResult{{
			ID:      rec.ToolUseID,
			Content: content,
			IsError: msg.IsError,
		}},
		RawIndex: lineNum,
		Tokens:   tokenizer.Estimate(content),
	}
}

// attachToolResult walks backwards through sess.Turns looking for assistant
// turns that issued matching tool_use_ids. Returns true if ALL tool results
// were attached.
func attachToolResult(sess *Session, turn Turn) bool {
	if len(turn.ToolResults) == 0 {
		return false
	}
	attached := 0
	for i := len(sess.Turns) - 1; i >= 0; i-- {
		for _, tc := range sess.Turns[i].ToolCalls {
			for _, tr := range turn.ToolResults {
				if tc.ID == tr.ID {
					est := tokenizer.Estimate(tr.Content)
					sess.Turns[i].ToolResults = append(sess.Turns[i].ToolResults, tr)
					sess.Turns[i].Tokens += est
					sess.Turns[i].ExtraLineIndices = append(sess.Turns[i].ExtraLineIndices, turn.RawIndex)
					sess.TokenCount += est
					sess.ToolResultTokens += est
					attached++
				}
			}
		}
	}
	return attached == len(turn.ToolResults)
}

func decodeMessage(raw []byte) (rawMessage, error) {
	var msg rawMessage
	if len(raw) == 0 {
		return msg, nil
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return msg, err
	}
	return msg, nil
}

func decodeBlocks(raw []byte) ([]rawBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var blocks []rawBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}
	return blocks, nil
}

// flattenContent handles either a plain string or an array of blocks.
func flattenContent(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	blocks, err := decodeBlocks(raw)
	if err != nil {
		return ""
	}
	var out string
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if out != "" {
				out += "\n\n"
			}
			out += bl.Text
		case "tool_result":
			if out != "" {
				out += "\n\n"
			}
			out += bl.Text
		}
	}
	return out
}

// turnSessionID extracts just the sessionId field without reparsing the
// whole record. Cheap enough to call once per line until we find one.
func turnSessionID(b []byte) string {
	var probe struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return ""
	}
	return probe.SessionID
}

// Compile-time guard so refactors that remove Parse don't silently break.
var _ interface {
	Parse(context.Context, string) (*Session, error)
} = (*Claude)(nil)
