package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/agentlens/internal/topics"
)

func RenderCheckboxRow(i int, t topics.Topic, selected, active bool, width int, s Styles) string {
	box := "[ ]"
	if selected {
		box = "[x]"
	}
	left := fmt.Sprintf("%s %d. %s", box, i+1, t.Label)
	right := s.Hint.Render(fmt.Sprintf("~%s", humanize.Comma(int64(t.TokenCount))))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + right
	if active {
		return lipgloss.NewStyle().Foreground(colText).Bold(true).Render(row)
	}
	return row
}

func RenderSelectionFooter(topics, turns, tokensFreed int) string {
	noun := "topics"
	if topics == 1 {
		noun = "topic"
	}
	return fmt.Sprintf("%d %s selected · ~%s tokens freed", topics, noun, humanize.Comma(int64(tokensFreed)))
}
