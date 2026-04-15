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
