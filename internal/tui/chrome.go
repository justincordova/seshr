package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// truncate shortens s to at most max runes. If truncation occurs the last
// character is replaced with an ellipsis (…). If max == 1 it always returns
// "…".
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// padRight pads s with spaces on the right until it reaches width. Strings
// already at or over width are returned unchanged.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// countLabel formats n with the singular label, pluralising with an "s" when
// n != 1. E.g. countLabel(1, "session") → "1 session", countLabel(3, "session") → "3 sessions".
func countLabel(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

// kbd renders a keyboard hint with a styled key and description.
func kbd(k, desc string) string {
	return keyStyle.Render(k) + " " + descStyle.Render(desc)
}

// joinHints joins multiple hint strings with a dim separator.
func joinHints(hints ...string) string {
	sep := dimStyle.Render("  ·  ")
	return strings.Join(hints, sep)
}

// hRule returns a full-width horizontal rule of the given width.
func hRule(width int) string {
	return dimStyle.Render(strings.Repeat("─", width))
}

// pill renders a small label badge with given foreground and background colors.
func pill(label string, fg, bg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(label)
}

// subviewHeader renders a chrome header row with title, optional breadcrumbs,
// and a right-aligned "esc back" hint, all on a mantle background.
func subviewHeader(width int, title string, crumbs []string) string {
	left := lipgloss.NewStyle().Foreground(colMauve).Bold(true).Render("◆ " + title)
	if len(crumbs) > 0 {
		sep := dimStyle.Render(" › ")
		crumbText := strings.Join(mapStyle(crumbs, subtitleStyle), sep)
		left = left + "  " + crumbText
	}
	right := dimStyle.Render("esc ") + keyStyle.Render("back")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	row := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Background(colMantle).Render(row)
}

// subviewFooter renders a chrome footer row with joined hints on a mantle background.
func subviewFooter(width int, hints ...string) string {
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Background(colMantle).Render(joinHints(hints...))
}

// subviewContent renders a padded content area with given dimensions.
func subviewContent(width, height int, body string) string {
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(body)
}

// panel renders a bordered box. When active is true, uses the active border color.
func panel(title, body string, width, height int, active bool) string {
	style := boxStyle
	if active {
		style = activeBoxStyle
	}
	content := body
	if title != "" {
		content = accentStyle.Render(title) + "\n" + body
	}
	return style.Width(width).Height(height).Render(content)
}

// mapStyle applies style to each string in the slice and returns the results.
func mapStyle(in []string, style lipgloss.Style) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = style.Render(s)
	}
	return out
}

