package session_test

import (
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/stretchr/testify/assert"
)

func TestNewClaude_ReturnsNonNil(t *testing.T) {
	// Arrange / Act
	p := claudeBackend.NewClaude()

	// Assert
	assert.NotNil(t, p)
}
