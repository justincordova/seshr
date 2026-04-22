package claude_test

import (
	"context"
	"os"
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaude_Parse_SimpleSession_ReturnsAllTurns(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Len(t, s.Turns, 4)
	assert.Equal(t, session.RoleUser, s.Turns[0].Role)
	assert.Contains(t, s.Turns[0].Content, "REST API")
	assert.Equal(t, session.RoleAssistant, s.Turns[1].Role)
	assert.Contains(t, s.Turns[1].Content, "framework")
	assert.Equal(t, session.SourceClaude, s.Source)
	assert.Equal(t, "sess-simple", s.ID)
}

func TestClaude_Parse_SimpleSession_UsesUsageTokens(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	// First assistant turn has usage input_tokens=12, output_tokens=7 → 19
	assert.Equal(t, 19, s.Turns[1].Tokens)
}

func TestClaude_Parse_RawIndexMatchesLineNumber(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	for i, turn := range s.Turns {
		assert.Equal(t, i, turn.RawIndex, "turn %d RawIndex", i)
	}
}

func TestClaude_Parse_MalformedLine_IsSkipped(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/malformed.jsonl")

	// Assert — 2 valid records survive, 1 garbage line is dropped, no error
	require.NoError(t, err)
	require.Len(t, s.Turns, 2)
	assert.Equal(t, session.RoleUser, s.Turns[0].Role)
	assert.Equal(t, session.RoleAssistant, s.Turns[1].Role)
}

func TestClaude_Parse_MultiTopic_AttachesToolResult(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/multi_topic.jsonl")

	// Assert
	require.NoError(t, err)
	// First assistant turn has one tool_use "t1"; its tool_result should be attached.
	var found bool
	for _, turn := range s.Turns {
		if turn.Role == session.RoleAssistant && len(turn.ToolCalls) > 0 && turn.ToolCalls[0].ID == "t1" {
			assert.Len(t, turn.ToolResults, 1, "tool result should attach to originating assistant turn")
			assert.Equal(t, "wrote package.json", turn.ToolResults[0].Content)
			found = true
		}
	}
	assert.True(t, found, "assistant turn with tool_use t1 not found")
}

func TestClaude_Parse_MultiTopic_ExtractsThinking(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/multi_topic.jsonl")

	// Assert
	require.NoError(t, err)
	var found bool
	for _, turn := range s.Turns {
		if turn.Thinking != "" {
			assert.Contains(t, turn.Thinking, "simple route")
			found = true
		}
	}
	assert.True(t, found, "thinking block should be extracted")
}

func TestClaude_Parse_NonExistentFile_ReturnsError(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	_, err := p.Parse(context.Background(), "/nonexistent/path.jsonl")

	// Assert
	require.Error(t, err)
}

func TestClaude_Parse_EmbeddedToolResult_AttachedToAssistant(t *testing.T) {
	p := claudeBackend.NewClaude()
	s, err := p.Parse(context.Background(), "../../../testdata/embedded_tool_results.jsonl")
	require.NoError(t, err)

	var bashTurn *session.Turn
	for i := range s.Turns {
		if len(s.Turns[i].ToolCalls) > 0 && s.Turns[i].ToolCalls[0].ID == "toolu_01" {
			bashTurn = &s.Turns[i]
			break
		}
	}
	require.NotNil(t, bashTurn, "should find assistant turn with tool_use toolu_01")
	require.Len(t, bashTurn.ToolResults, 1, "embedded tool result should be attached")
	assert.Equal(t, "file1.txt\nfile2.txt", bashTurn.ToolResults[0].Content)
	assert.False(t, bashTurn.ToolResults[0].IsError)
}

func TestClaude_Parse_EmbeddedToolResult_MultipleInOneRecord(t *testing.T) {
	p := claudeBackend.NewClaude()
	s, err := p.Parse(context.Background(), "../../../testdata/embedded_tool_results.jsonl")
	require.NoError(t, err)

	var readTurn *session.Turn
	for i := range s.Turns {
		if len(s.Turns[i].ToolCalls) > 0 && s.Turns[i].ToolCalls[0].ID == "toolu_02" {
			readTurn = &s.Turns[i]
			break
		}
	}
	require.NotNil(t, readTurn, "should find assistant turn with tool_use toolu_02")
	require.Len(t, readTurn.ToolResults, 2, "both tool results should attach")
	assert.Equal(t, "hello world", readTurn.ToolResults[0].Content)
	assert.Equal(t, "goodbye world", readTurn.ToolResults[1].Content)
}

func TestClaude_Parse_EmbeddedToolResult_ErrorResult(t *testing.T) {
	p := claudeBackend.NewClaude()
	s, err := p.Parse(context.Background(), "../../../testdata/embedded_tool_results.jsonl")
	require.NoError(t, err)

	var errTurn *session.Turn
	for i := range s.Turns {
		if len(s.Turns[i].ToolCalls) > 0 && s.Turns[i].ToolCalls[0].ID == "toolu_04" {
			errTurn = &s.Turns[i]
			break
		}
	}
	require.NotNil(t, errTurn, "should find assistant turn with tool_use toolu_04")
	require.Len(t, errTurn.ToolResults, 1)
	assert.Equal(t, "Exit code 1", errTurn.ToolResults[0].Content)
	assert.True(t, errTurn.ToolResults[0].IsError)
}

func TestClaude_Parse_EmbeddedToolResult_NoOrphanUserTurns(t *testing.T) {
	p := claudeBackend.NewClaude()
	s, err := p.Parse(context.Background(), "../../../testdata/embedded_tool_results.jsonl")
	require.NoError(t, err)

	for _, turn := range s.Turns {
		if turn.Role == session.RoleUser {
			assert.NotEmpty(t, turn.Content, "user turns with only tool_results should not appear as empty user turns")
		}
	}
}

func TestClaude_Parse_CompactBoundary_Detected(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/compact_boundary.jsonl")

	// Assert
	require.NoError(t, err)
	require.Len(t, s.CompactBoundaries, 1)
	cb := s.CompactBoundaries[0]
	assert.Equal(t, 2, cb.TurnIndex, "boundary should point to first turn after compaction")
	assert.Equal(t, "manual", cb.Trigger)
	assert.Equal(t, 141000, cb.PreTokens)
	assert.Equal(t, 142000, cb.DurationMs)
}

func TestClaude_Parse_CompactContinuation_Marked(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/compact_boundary.jsonl")

	// Assert
	require.NoError(t, err)
	var found bool
	for _, turn := range s.Turns {
		if turn.IsCompactContinuation {
			found = true
			assert.Equal(t, session.RoleUser, turn.Role)
			assert.Contains(t, turn.Content, "This session is being continued")
		}
	}
	assert.True(t, found, "one turn should be marked as compact continuation")
}

func TestClaude_Parse_NoCompactBoundary_EmptySlice(t *testing.T) {
	// Arrange
	p := claudeBackend.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	assert.Empty(t, s.CompactBoundaries)
}

func TestClaude_Parse_EmbeddedToolResult_BlockArrayContent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jsonl"
	input := `{"type":"user","message":{"role":"user","content":"go"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"uuid":"a1","timestamp":"2025-01-01T00:00:00Z","sessionId":"s"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]}]}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))

	p := claudeBackend.NewClaude()
	s, err := p.Parse(context.Background(), path)
	require.NoError(t, err)

	var found *session.Turn
	for i := range s.Turns {
		if len(s.Turns[i].ToolCalls) > 0 && s.Turns[i].ToolCalls[0].ID == "t1" {
			found = &s.Turns[i]
		}
	}
	require.NotNil(t, found)
	require.Len(t, found.ToolResults, 1)
	assert.Contains(t, found.ToolResults[0].Content, "line one")
	assert.Contains(t, found.ToolResults[0].Content, "line two")
}
