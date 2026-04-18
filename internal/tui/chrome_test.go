package tui

import (
	"strings"
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

func TestWrapHints(t *testing.T) {
	hints := []string{"a key", "b other", "c more"}
	lines := wrapHints(hints, 10, "   ")
	assert.Len(t, lines, 3)
}

func TestWrapHints_FitsOnOneLine(t *testing.T) {
	hints := []string{"short", "tiny"}
	lines := wrapHints(hints, 100, "   ")
	assert.Len(t, lines, 1)
}

func TestWrapHints_Empty(t *testing.T) {
	lines := wrapHints(nil, 80, "   ")
	assert.Nil(t, lines)
}

func TestPill(t *testing.T) {
	got := pill("tag", lipgloss.AdaptiveColor{Dark: "#cba6f7", Light: "#8839ef"}, lipgloss.AdaptiveColor{Dark: "#313244", Light: "#ccd0da"})
	assert.NotEmpty(t, got)
}

func TestContentWidth_ClampsAtCap(t *testing.T) {
	assert.Equal(t, 80, contentWidth(80))
	assert.Equal(t, 100, contentWidth(100))
	assert.Equal(t, 100, contentWidth(200))
}

func TestCenterBlock_AddsLeftMargin(t *testing.T) {
	out := centerBlock("abc", 120)
	assert.Equal(t, strings.Repeat(" ", 10)+"abc", out)
}

func TestCenterBlock_NarrowTerminalNoop(t *testing.T) {
	out := centerBlock("abc", 80)
	assert.Equal(t, "abc", out)
}

func TestCenterBlock_MultipleLines(t *testing.T) {
	out := centerBlock("a\nbb", 120)
	pad := strings.Repeat(" ", 10)
	assert.Equal(t, pad+"a\n"+pad+"bb", out)
}
