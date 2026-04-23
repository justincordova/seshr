package opencode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/justincordova/seshr/internal/session"
)

// partRow is a narrow row struct for the `part` table. The data column is
// JSON that we decode lazily via json_extract or by unmarshaling into the
// per-type payload structs below.
type partRow struct {
	ID          string
	MessageID   string
	SessionID   string
	TimeCreated int64 // Unix milliseconds
	Data        json.RawMessage
}

// messageRow is a narrow row struct for the `message` table. Role and
// parentID live in the `data` JSON (not as columns) — decoded into
// messageEnvelope for chain-walking.
type messageRow struct {
	ID          string
	SessionID   string
	TimeCreated int64 // Unix milliseconds
	Data        json.RawMessage
}

// messageEnvelope captures the fields we need from message.data JSON. OC
// stores the full message (user or assistant) as a blob; role, parent link,
// and tokens/cost on assistant messages are what seshr consumes.
type messageEnvelope struct {
	Role     string          `json:"role"`
	ParentID string          `json:"parentID"`
	Tokens   messageTokens   `json:"tokens"`
	Cost     float64         `json:"cost"`
	Time     messageTimeInfo `json:"time"`
}

type messageTokens struct {
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	Cache     struct {
		Read  int `json:"read"`
		Write int `json:"write"`
	} `json:"cache"`
}

// Total returns input + output + reasoning + cache.read + cache.write. Cache
// reads count against the displayed aggregate — they're still billable tokens
// from the user's quota perspective, just cheaper.
func (t messageTokens) Total() int {
	return t.Input + t.Output + t.Reasoning + t.Cache.Read + t.Cache.Write
}

type messageTimeInfo struct {
	Created   int64 `json:"created"`
	Completed int64 `json:"completed"`
}

// partText captures the shape of `{"type":"text","text":"...","time":{...}}`.
type partText struct {
	Text string `json:"text"`
}

// partReasoning captures `{"type":"reasoning","text":"...","time":{...}}`.
type partReasoning struct {
	Text string `json:"text"`
}

// partTool captures the subset of tool parts we render. `state.status` is
// one of completed | error | running | pending. `state.input`/`state.output`
// are free-form — we stringify for display without decoding.
type partTool struct {
	CallID string        `json:"callID"`
	Tool   string        `json:"tool"`
	State  partToolState `json:"state"`
}

type partToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

// partPatch is rendered like text. OC uses patch parts for inline diff
// summaries the agent emits during edit operations.
type partPatch struct {
	Text string `json:"text"`
	// More fields exist on the wire; we only surface the textual summary.
}

// partFile captures `{"type":"file","filename":"...","url":"...",...}`.
// Displayed as a synthetic tool_result so the turn renderer shows it.
type partFile struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Mime     string `json:"mime"`
}

// partCompaction is a marker-only part: `{"type":"compaction","auto":false}`.
// Presence is all we need; Trigger is derived from Auto.
type partCompaction struct {
	Auto bool `json:"auto"`
}

// decodedChain is the aggregated output of walking a chain of messages +
// parts. Turns are in chronological order (root → leaf).
type decodedChain struct {
	Turns            []session.Turn
	Boundaries       []session.CompactBoundary
	ToolResultTokens int
	// TotalTokens is the sum of messageTokens.Total() across all assistant
	// messages in the chain. Populated so callers can set Session.TokenCount.
	TotalTokens int
}

// decodeChain translates a (chain, parts) pair into session turns + compact
// boundaries. Caller guarantees messages are in chronological order and
// parts are grouped/ordered by (message_id, time_created, id). Any
// unrecognized role or part type logs a warn and is skipped.
func decodeChain(messages []messageRow, parts []partRow) (decodedChain, error) {
	// Index parts by message_id so we can walk them per message in order.
	byMsg := make(map[string][]partRow, len(messages))
	for _, p := range parts {
		byMsg[p.MessageID] = append(byMsg[p.MessageID], p)
	}

	out := decodedChain{}
	for _, msg := range messages {
		env, err := decodeEnvelope(msg.Data)
		if err != nil {
			slog.Warn("opencode: skipping message with undecodable envelope",
				"session", msg.SessionID, "msg", msg.ID, "err", err)
			continue
		}
		role, ok := mapRole(env.Role)
		if !ok {
			slog.Warn("opencode: unknown message role; skipping",
				"session", msg.SessionID, "msg", msg.ID, "role", env.Role)
			continue
		}

		turn := session.Turn{
			Role:      role,
			Timestamp: time.UnixMilli(msg.TimeCreated),
			Tokens:    env.Tokens.Total(),
		}
		if role == session.RoleAssistant {
			out.TotalTokens += env.Tokens.Total()
		}

		for _, p := range byMsg[msg.ID] {
			if err := applyPart(&turn, p, &out); err != nil {
				slog.Warn("opencode: failed to decode part; skipping",
					"session", msg.SessionID, "part", p.ID, "err", err)
			}
		}

		// A compaction part lives on its own synthetic "message" in some OC
		// versions — handle the chain-level boundary insertion after the
		// per-turn emit when we detect one via applyPart. applyPart appends
		// boundaries to out.Boundaries with the correct turn index.

		out.Turns = append(out.Turns, turn)
	}
	return out, nil
}

// applyPart mutates turn and out according to the part's type. A completed
// tool part produces both a ToolCall and a paired ToolResult; a running or
// pending tool produces only the call. Compaction parts produce a boundary
// at the current turn's index (the first turn AFTER the boundary is the
// next turn appended — matches session.CompactBoundary.TurnIndex semantics).
func applyPart(turn *session.Turn, p partRow, out *decodedChain) error {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(p.Data, &head); err != nil {
		return fmt.Errorf("decode part head: %w", err)
	}
	switch head.Type {
	case "text":
		var t partText
		if err := json.Unmarshal(p.Data, &t); err != nil {
			return fmt.Errorf("decode text: %w", err)
		}
		turn.Content = appendText(turn.Content, t.Text)
	case "reasoning":
		var r partReasoning
		if err := json.Unmarshal(p.Data, &r); err != nil {
			return fmt.Errorf("decode reasoning: %w", err)
		}
		turn.Thinking = appendText(turn.Thinking, r.Text)
	case "patch":
		var pp partPatch
		if err := json.Unmarshal(p.Data, &pp); err != nil {
			return fmt.Errorf("decode patch: %w", err)
		}
		turn.Content = appendText(turn.Content, pp.Text)
	case "tool":
		var tp partTool
		if err := json.Unmarshal(p.Data, &tp); err != nil {
			return fmt.Errorf("decode tool: %w", err)
		}
		turn.ToolCalls = append(turn.ToolCalls, session.ToolCall{
			ID:    tp.CallID,
			Name:  tp.Tool,
			Input: []byte(tp.State.Input),
		})
		switch tp.State.Status {
		case "completed":
			turn.ToolResults = append(turn.ToolResults, session.ToolResult{
				ID:      tp.CallID,
				Content: string(tp.State.Output),
			})
			out.ToolResultTokens += estimateTokens(tp.State.Output)
		case "error":
			turn.ToolResults = append(turn.ToolResults, session.ToolResult{
				ID:      tp.CallID,
				Content: string(tp.State.Output),
				IsError: true,
			})
			out.ToolResultTokens += estimateTokens(tp.State.Output)
		case "running", "pending":
			// Call only; no result yet.
		default:
			slog.Warn("opencode: unknown tool-part status",
				"status", tp.State.Status, "call", tp.CallID)
		}
	case "file":
		var pf partFile
		if err := json.Unmarshal(p.Data, &pf); err != nil {
			return fmt.Errorf("decode file: %w", err)
		}
		// Surface as a synthetic tool_result on the owning turn. Content is
		// the URL/filename — inline contents live at pf.URL (not fetched).
		turn.ToolResults = append(turn.ToolResults, session.ToolResult{
			ID:      p.ID,
			Content: fmt.Sprintf("[file] %s (%s)", pf.Filename, pf.Mime),
		})
	case "compaction":
		// TurnIndex is the index of the first turn AFTER the boundary —
		// that's the next turn we're about to append, so len(out.Turns).
		var pc partCompaction
		_ = json.Unmarshal(p.Data, &pc)
		trigger := "manual"
		if pc.Auto {
			trigger = "auto"
		}
		out.Boundaries = append(out.Boundaries, session.CompactBoundary{
			TurnIndex: len(out.Turns),
			Trigger:   trigger,
		})
	case "step-start", "step-finish":
		// Bookkeeping only; nothing renders.
	case "agent", "subtask":
		// TODO(v1.1): render subagent tree. Drop for v1.
		slog.Debug("opencode: dropping subagent part for v1",
			"type", head.Type, "part", p.ID)
	default:
		slog.Debug("opencode: ignoring unknown part type",
			"type", head.Type, "part", p.ID)
	}
	return nil
}

// decodeEnvelope unmarshals the top-level message.data envelope. Errors
// bubble up so the caller can skip the message.
func decodeEnvelope(raw json.RawMessage) (messageEnvelope, error) {
	var env messageEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return messageEnvelope{}, err
	}
	return env, nil
}

// mapRole converts OC's role string to session.Role. Returns (Role, true)
// for recognized values; (zero, false) otherwise.
func mapRole(r string) (session.Role, bool) {
	switch r {
	case "user":
		return session.RoleUser, true
	case "assistant":
		return session.RoleAssistant, true
	}
	return "", false
}

// appendText joins existing + new with a single blank line separator when
// both are non-empty. Mirrors how Claude's parser collapses multiple content
// blocks within one assistant turn.
func appendText(existing, add string) string {
	if add == "" {
		return existing
	}
	if existing == "" {
		return add
	}
	return existing + "\n\n" + add
}

// estimateTokens returns a rough token count for a JSON blob. We use the
// rune/3.5 heuristic from the tokenizer package; this is only used for the
// aggregate ToolResultTokens display and doesn't need to be exact.
func estimateTokens(b json.RawMessage) int {
	// Same coarse heuristic the Claude scanner uses.
	return len(b) / 4
}
