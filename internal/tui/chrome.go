package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Truncate shortens s to at most max runes. If truncation occurs the last
// character is replaced with an ellipsis (…). If max == 1 it always returns
// "…".
func Truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// PadRight pads s with spaces on the right until it reaches width. Strings
// already at or over width are returned unchanged.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// CountLabel formats n with the singular label, pluralising with an "s" when
// n != 1. E.g. CountLabel(1, "session") → "1 session", CountLabel(3, "session") → "3 sessions".
func CountLabel(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

// Kbd renders a keyboard hint with a styled key and description.
func Kbd(k, desc string) string {
	return keyStyle.Render(k) + " " + descStyle.Render(desc)
}

// JoinHints joins multiple hint strings with a dim separator.
func JoinHints(hints ...string) string {
	sep := dimStyle.Render("  ·  ")
	return strings.Join(hints, sep)
}

// HRule returns a full-width horizontal rule of the given width.
func HRule(width int) string {
	return dimStyle.Render(strings.Repeat("─", width))
}

// Pill renders a small label badge with given foreground and background hex colors.
func Pill(label, fg, bg string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(lipgloss.Color(bg)).
		Padding(0, 1).
		Render(label)
}

// SubviewHeader renders a chrome header row with title, optional breadcrumbs,
// and a right-aligned "esc back" hint, all on a mantle background.
func SubviewHeader(width int, title string, crumbs []string) string {
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

// SubviewFooter renders a chrome footer row with joined hints on a mantle background.
func SubviewFooter(width int, hints ...string) string {
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Background(colMantle).Render(JoinHints(hints...))
}

// SubviewContent renders a padded content area with given dimensions.
func SubviewContent(width, height int, body string) string {
	return lipgloss.NewStyle().Width(width).Height(height).Padding(1, 2).Render(body)
}

// Panel renders a bordered box. When active is true, uses the active border color.
func Panel(title, body string, width, height int, active bool) string {
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

// Expose package-level style vars for use by chrome consumers that reference
// the unexported vars indirectly through these sentinel uses.
var (
	_ = colBase
	_ = colCrust
	_ = colSurface2
	_ = colOverlay1
	_ = colSubtext1
	_ = colMaroon
	_ = colPeach
	_ = colTeal
	_ = colSky
	_ = colBlue
	_ = colFlamingo
	_ = textStyle
	_ = subtleStyle
	_ = successStyle
	_ = warningStyle
	_ = errorStyle
	_ = selectedRowStyle
)
