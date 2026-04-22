package session_test

import (
	"testing"

	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestNewClaude_ReturnsNonNil(t *testing.T) {
	// Arrange / Act
	p := session.NewClaude()

	// Assert
	assert.NotNil(t, p)
}
