package parser_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/parser"
	"github.com/stretchr/testify/assert"
)

func TestNewClaude_ReturnsNonNil(t *testing.T) {
	// Arrange / Act
	p := parser.NewClaude()

	// Assert
	assert.NotNil(t, p)
}
