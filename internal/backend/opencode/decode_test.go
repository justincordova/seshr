package opencode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justincordova/seshr/internal/session"
)

// partWith constructs a partRow helper.
func partWith(id, msgID string, payload any) partRow {
	raw, _ := json.Marshal(payload)
	return partRow{ID: id, MessageID: msgID, Data: raw}
}

func TestDecodeChain_TextPart_AppendsToContent(t *testing.T) {
	msgs := []messageRow{msg("u1", "user", "", 1000)}
	parts := []partRow{partWith("p1", "u1", map[string]any{"type": "text", "text": "hello"})}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)
	require.Len(t, dc.Turns, 1)

	assert.Equal(t, "hello", dc.Turns[0].Content)
	assert.Equal(t, session.RoleUser, dc.Turns[0].Role)
}

func TestDecodeChain_ReasoningPart_AppendsToThinking(t *testing.T) {
	msgs := []messageRow{msg("a1", "assistant", "u1", 2000)}
	parts := []partRow{partWith("p1", "a1", map[string]any{"type": "reasoning", "text": "thinking..."})}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)

	assert.Equal(t, "thinking...", dc.Turns[0].Thinking)
}

func TestDecodeChain_CompletedTool_EmitsCallAndResult(t *testing.T) {
	msgs := []messageRow{msg("a1", "assistant", "u1", 2000)}
	parts := []partRow{partWith("p1", "a1", map[string]any{
		"type":   "tool",
		"callID": "call_abc",
		"tool":   "bash",
		"state": map[string]any{
			"status": "completed",
			"input":  map[string]any{"cmd": "ls"},
			"output": "file1",
		},
	})}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)
	require.Len(t, dc.Turns, 1)

	require.Len(t, dc.Turns[0].ToolCalls, 1)
	assert.Equal(t, "bash", dc.Turns[0].ToolCalls[0].Name)
	require.Len(t, dc.Turns[0].ToolResults, 1)
	assert.False(t, dc.Turns[0].ToolResults[0].IsError)
}

func TestDecodeChain_RunningTool_CallOnly(t *testing.T) {
	msgs := []messageRow{msg("a1", "assistant", "u1", 2000)}
	parts := []partRow{partWith("p1", "a1", map[string]any{
		"type":   "tool",
		"callID": "call_r",
		"tool":   "bash",
		"state":  map[string]any{"status": "running", "input": map[string]any{"cmd": "sleep"}},
	})}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)

	assert.Len(t, dc.Turns[0].ToolCalls, 1)
	assert.Empty(t, dc.Turns[0].ToolResults, "running tools emit no result")
}

func TestDecodeChain_ErrorTool_EmitsErrorResult(t *testing.T) {
	msgs := []messageRow{msg("a1", "assistant", "u1", 2000)}
	parts := []partRow{partWith("p1", "a1", map[string]any{
		"type":   "tool",
		"callID": "call_e",
		"tool":   "bash",
		"state": map[string]any{
			"status": "error",
			"input":  map[string]any{"cmd": "asdf"},
			"output": "command not found",
		},
	})}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)

	require.Len(t, dc.Turns[0].ToolResults, 1)
	assert.True(t, dc.Turns[0].ToolResults[0].IsError)
}

func TestDecodeChain_CompactionPart_EmitsBoundary(t *testing.T) {
	msgs := []messageRow{
		msg("u1", "user", "", 1000),
		msg("a1", "assistant", "u1", 2000),
	}
	// Compaction on the second message; boundary TurnIndex == 1 (index of
	// the assistant turn, which is the first post-compaction turn since we
	// capture the boundary BEFORE appending the assistant turn).
	parts := []partRow{
		partWith("p1", "u1", map[string]any{"type": "text", "text": "hi"}),
		partWith("p2", "a1", map[string]any{"type": "compaction", "auto": false}),
		partWith("p3", "a1", map[string]any{"type": "text", "text": "post"}),
	}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)

	require.Len(t, dc.Boundaries, 1)
	assert.Equal(t, 1, dc.Boundaries[0].TurnIndex)
	assert.Equal(t, "manual", dc.Boundaries[0].Trigger)
}

func TestDecodeChain_UnknownRole_SkippedWithWarn(t *testing.T) {
	// system-role message is not yet rendered; skipped.
	msgs := []messageRow{msg("s1", "system", "", 500)}

	dc, err := decodeChain(msgs, nil)
	require.NoError(t, err)

	assert.Empty(t, dc.Turns)
}

func TestDecodeChain_IgnoredPartTypes(t *testing.T) {
	msgs := []messageRow{msg("a1", "assistant", "u1", 2000)}
	// step-start/step-finish are bookkeeping; produce nothing.
	parts := []partRow{
		partWith("p1", "a1", map[string]any{"type": "step-start"}),
		partWith("p2", "a1", map[string]any{"type": "step-finish"}),
	}

	dc, err := decodeChain(msgs, parts)
	require.NoError(t, err)

	assert.Empty(t, dc.Turns[0].Content)
	assert.Empty(t, dc.Turns[0].ToolCalls)
}

func TestMessageTokens_Total_SumsAllFields(t *testing.T) {
	tokens := messageTokens{Input: 10, Output: 20, Reasoning: 5}
	tokens.Cache.Read = 100
	tokens.Cache.Write = 0

	assert.Equal(t, 135, tokens.Total())
}

func TestCursor_RoundTrip_PreservesFields(t *testing.T) {
	original := cursorData{LastMessageID: "msg_x", LastTimeCreated: 123456}

	decoded, err := decodeCursor(encodeCursor(original))

	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeCursor_KindMismatch_Error(t *testing.T) {
	// Fabricate a Claude cursor and try to decode it as OC.
	bad := encodeCursor(cursorData{LastMessageID: "x", LastTimeCreated: 1})
	bad.Kind = session.SourceClaude

	_, err := decodeCursor(bad)

	assert.Error(t, err)
}
