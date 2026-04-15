package tokenizer_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/tokenizer"
	"github.com/stretchr/testify/assert"
)

func TestEstimate_EmptyString_ReturnsZero(t *testing.T) {
	// Arrange / Act
	got := tokenizer.Estimate("")

	// Assert
	assert.Equal(t, 0, got)
}
