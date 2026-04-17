package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"short string unchanged", "hello", 10, "hello"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"long string truncated", "hello world", 8, "hello w…"},
		{"width=1 returns ellipsis", "abc", 1, "…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := truncate(tc.input, tc.max)
			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPadRightRaw(t *testing.T) {
	cases := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{name: "shorter than width", input: "ab", width: 5, want: "ab   "},
		{name: "exact width", input: "abc", width: 3, want: "abc"},
		{name: "longer than width", input: "abcde", width: 3, want: "abcde"},
		{name: "empty string", input: "", width: 3, want: "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := padRightRaw(tc.input, tc.width)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCountLabel(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		singular string
		wantSep  bool // true if we expect plural (not singular form)
	}{
		{"zero is plural", 0, "item", false},
		{"one is singular", 1, "item", false},
		{"two is plural", 2, "item", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := countLabel(tc.n, tc.singular)
			// Assert
			assert.Contains(t, got, tc.singular)
			if tc.n == 1 {
				assert.NotContains(t, got, tc.singular+"s")
			}
			if tc.n != 1 {
				assert.Contains(t, got, tc.singular+"s")
			}
		})
	}
}

func TestKbd(t *testing.T) {
	// Arrange
	k := "q"
	desc := "quit"

	// Act
	got := kbd(k, desc)

	// Assert
	assert.Contains(t, got, k)
	assert.Contains(t, got, desc)
}

func TestJoinHints(t *testing.T) {
	t.Run("single hint has no separator", func(t *testing.T) {
		// Act
		got := joinHints("quit")
		// Assert
		assert.NotContains(t, got, "·")
	})

	t.Run("multiple hints have separator", func(t *testing.T) {
		// Act
		got := joinHints("nav", "open", "quit")
		// Assert
		assert.Contains(t, got, "·")
	})
}

func TestPill(t *testing.T) {
	// Act
	got := pill("tag", lipgloss.AdaptiveColor{Dark: "#cba6f7", Light: "#8839ef"}, lipgloss.AdaptiveColor{Dark: "#313244", Light: "#ccd0da"})

	// Assert
	assert.NotEmpty(t, got)
}
