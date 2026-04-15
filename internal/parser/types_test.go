package parser_test

import (
	"testing"
	"time"

	"github.com/justincordova/agentlens/internal/parser"
	"github.com/stretchr/testify/assert"
)

func TestSession_ZeroValue_HasUsableDefaults(t *testing.T) {
	// Arrange / Act
	s := parser.Session{}

	// Assert
	assert.Empty(t, s.ID)
	assert.Empty(t, s.Source)
	assert.Zero(t, s.CreatedAt)
	assert.Zero(t, s.ModifiedAt)
	assert.Equal(t, 0, s.TokenCount)
	assert.Nil(t, s.Turns)
	assert.Nil(t, s.ChainedFiles)
}

func TestTurn_ZeroValue_HasUsableDefaults(t *testing.T) {
	// Arrange / Act
	turn := parser.Turn{}

	// Assert
	assert.Empty(t, turn.Role)
	assert.Equal(t, time.Time{}, turn.Timestamp)
	assert.Empty(t, turn.Content)
	assert.Nil(t, turn.ToolCalls)
	assert.Nil(t, turn.ToolResults)
	assert.Empty(t, turn.Thinking)
	assert.Equal(t, 0, turn.RawIndex)
	assert.Equal(t, 0, turn.Tokens)
}
