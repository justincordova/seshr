package parser_test

import (
	"context"
	"testing"

	"github.com/justincordova/agentlens/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaude_Parse_SimpleSession_ReturnsAllTurns(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Len(t, s.Turns, 4)
	assert.Equal(t, parser.RoleUser, s.Turns[0].Role)
	assert.Contains(t, s.Turns[0].Content, "REST API")
	assert.Equal(t, parser.RoleAssistant, s.Turns[1].Role)
	assert.Contains(t, s.Turns[1].Content, "framework")
	assert.Equal(t, parser.SourceClaude, s.Source)
	assert.Equal(t, "sess-simple", s.ID)
}

func TestClaude_Parse_SimpleSession_UsesUsageTokens(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	// First assistant turn has usage input_tokens=12, output_tokens=7 → 19
	assert.Equal(t, 19, s.Turns[1].Tokens)
}

func TestClaude_Parse_RawIndexMatchesLineNumber(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/simple.jsonl")

	// Assert
	require.NoError(t, err)
	for i, turn := range s.Turns {
		assert.Equal(t, i, turn.RawIndex, "turn %d RawIndex", i)
	}
}

func TestClaude_Parse_MalformedLine_IsSkipped(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/malformed.jsonl")

	// Assert — 2 valid records survive, 1 garbage line is dropped, no error
	require.NoError(t, err)
	require.Len(t, s.Turns, 2)
	assert.Equal(t, parser.RoleUser, s.Turns[0].Role)
	assert.Equal(t, parser.RoleAssistant, s.Turns[1].Role)
}

func TestClaude_Parse_MultiTopic_AttachesToolResult(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/multi_topic.jsonl")

	// Assert
	require.NoError(t, err)
	// First assistant turn has one tool_use "t1"; its tool_result should be attached.
	var found bool
	for _, turn := range s.Turns {
		if turn.Role == parser.RoleAssistant && len(turn.ToolCalls) > 0 && turn.ToolCalls[0].ID == "t1" {
			assert.Len(t, turn.ToolResults, 1, "tool result should attach to originating assistant turn")
			assert.Equal(t, "wrote package.json", turn.ToolResults[0].Content)
			found = true
		}
	}
	assert.True(t, found, "assistant turn with tool_use t1 not found")
}

func TestClaude_Parse_MultiTopic_ExtractsThinking(t *testing.T) {
	// Arrange
	p := parser.NewClaude()

	// Act
	s, err := p.Parse(context.Background(), "../../testdata/multi_topic.jsonl")

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
	p := parser.NewClaude()

	// Act
	_, err := p.Parse(context.Background(), "/nonexistent/path.jsonl")

	// Assert
	require.Error(t, err)
}
