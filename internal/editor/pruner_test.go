package editor_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/editor"
	"github.com/stretchr/testify/assert"
)

func TestNewPruner_ReturnsNonNil(t *testing.T) {
	// Arrange / Act
	p := editor.NewPruner()

	// Assert
	assert.NotNil(t, p)
}
