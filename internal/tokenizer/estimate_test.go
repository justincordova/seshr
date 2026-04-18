package tokenizer_test

import (
	"strings"
	"testing"

	"github.com/justincordova/seshr/internal/tokenizer"
	"github.com/stretchr/testify/assert"
)

func TestEstimate_EmptyString_ReturnsZero(t *testing.T) {
	// Arrange / Act
	got := tokenizer.Estimate("")

	// Assert
	assert.Equal(t, 0, got)
}

func TestEstimate_ShortString_RoundsUp(t *testing.T) {
	// Arrange — 10 chars / 3.5 = 2.857 → round to 3
	in := "abcdefghij"

	// Act
	got := tokenizer.Estimate(in)

	// Assert
	assert.Equal(t, 3, got)
}

func TestEstimate_LongString_Scales(t *testing.T) {
	// Arrange — 3500 chars / 3.5 = 1000
	in := strings.Repeat("x", 3500)

	// Act
	got := tokenizer.Estimate(in)

	// Assert
	assert.Equal(t, 1000, got)
}

func TestFromUsage_InputPlusOutput_Sums(t *testing.T) {
	// Arrange
	u := tokenizer.Usage{InputTokens: 120, OutputTokens: 45}

	// Act
	got := tokenizer.FromUsage(u)

	// Assert
	assert.Equal(t, 165, got)
}

func TestFromUsage_IncludesCacheInputs(t *testing.T) {
	// Arrange — all four input fields count as billable input context
	u := tokenizer.Usage{
		InputTokens:              10,
		CacheCreationInputTokens: 100,
		CacheReadInputTokens:     200,
		OutputTokens:             5,
	}

	// Act
	got := tokenizer.FromUsage(u)

	// Assert
	assert.Equal(t, 315, got)
}
