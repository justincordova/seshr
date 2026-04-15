package tui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/agentlens/internal/tui"
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
			got := tui.Truncate(tc.input, tc.max)
			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
	}{
		{"short string padded to width", "hi", 10},
		{"exact width unchanged", "hello", 5},
		{"over width unchanged", "hello world", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := tui.PadRight(tc.input, tc.width)
			// Assert
			if len(tc.input) < tc.width {
				assert.Equal(t, tc.width, len(got))
			} else {
				assert.Equal(t, tc.input, got)
			}
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
			got := tui.CountLabel(tc.n, tc.singular)
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
	key := "q"
	desc := "quit"

	// Act
	got := tui.Kbd(key, desc)

	// Assert
	assert.Contains(t, got, key)
	assert.Contains(t, got, desc)
}

func TestJoinHints(t *testing.T) {
	t.Run("single hint has no separator", func(t *testing.T) {
		// Act
		got := tui.JoinHints("quit")
		// Assert
		assert.NotContains(t, got, "·")
	})

	t.Run("multiple hints have separator", func(t *testing.T) {
		// Act
		got := tui.JoinHints("nav", "open", "quit")
		// Assert
		assert.Contains(t, got, "·")
	})
}

func TestHRule(t *testing.T) {
	tests := []struct {
		name  string
		width int
	}{
		{"width 10", 10},
		{"width 80", 80},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := tui.HRule(tc.width)
			// Assert — rendered width should match
			assert.Equal(t, tc.width, lipgloss.Width(got))
		})
	}
}

func TestPill(t *testing.T) {
	// Act
	got := tui.Pill("status", "#f38ba8", "#1e1e2e")

	// Assert
	assert.NotEmpty(t, got)
	assert.True(t, strings.Contains(got, "status"))
}

func TestSubviewHeader(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		crumbs []string
	}{
		{"title only", "Overview", nil},
		{"title with crumbs", "Overview", []string{"proj-a", "session-1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := tui.SubviewHeader(80, tc.title, tc.crumbs)
			// Assert
			assert.Contains(t, got, tc.title)
		})
	}
}

func TestSubviewFooter(t *testing.T) {
	// Act
	got := tui.SubviewFooter(80, "j/k navigate", "q quit")

	// Assert
	assert.NotEmpty(t, got)
}
