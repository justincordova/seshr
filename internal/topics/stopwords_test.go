package topics_test

import (
	"testing"

	"github.com/justincordova/seshly/internal/topics"
	"github.com/stretchr/testify/assert"
)

func TestIsStopword_CommonWords_ReturnsTrue(t *testing.T) {
	cases := []string{"the", "and", "of", "to", "a", "in", "is", "that", "for", "it"}
	for _, w := range cases {
		assert.True(t, topics.IsStopword(w), "expected %q to be a stopword", w)
	}
}

func TestIsStopword_ContentWords_ReturnsFalse(t *testing.T) {
	cases := []string{"auth", "middleware", "rate", "jwt", "express", "database"}
	for _, w := range cases {
		assert.False(t, topics.IsStopword(w), "expected %q not to be a stopword", w)
	}
}

func TestIsStopword_CaseInsensitive(t *testing.T) {
	assert.True(t, topics.IsStopword("The"))
	assert.True(t, topics.IsStopword("AND"))
}
