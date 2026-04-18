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

// padRightRaw pads s with spaces on the right until it reaches width runes.
// Strings already at or over width are returned unchanged. Intended for
// padding raw (unstyled) text before applying lipgloss styles — padding a
// pre-rendered string with %-Ns would count ANSI escape bytes.
func padRightRaw(s string, width int) string {
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

// kbdPill renders a keyboard hint with the key in a surface-colored pill.
func kbdPill(k, desc string) string {
	keyPill := lipgloss.NewStyle().
		Foreground(colText).
		Background(colSurface0).
		Padding(0, 1).
		Render(k)
	return keyPill + descStyle.Render(" "+desc)
}

// joinHints joins multiple hint strings with a dim separator.
func joinHints(hints ...string) string {
	sep := dimStyle.Render("  ·  ")
	return strings.Join(hints, sep)
}

// wrapHints packs hint strings into lines that each fit within width.
// Each hint is treated as atomic — never split across lines.
func wrapHints(hints []string, width int, sep string) []string {
	if len(hints) == 0 {
		return nil
	}
	var lines []string
	var cur string
	for _, h := range hints {
		if cur == "" {
			cur = h
			continue
		}
		candidate := cur + sep + h
		if lipgloss.Width(candidate) > width {
			lines = append(lines, cur)
			cur = h
		} else {
			cur = candidate
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// renderCenteredFooter wraps hints into centered lines.
func renderCenteredFooter(hints []string, width int) string {
	sep := dimStyle.Render("  ·  ")
	lines := wrapHints(hints, width-4, sep)
	var b strings.Builder
	for i, line := range lines {
		gap := (width - lipgloss.Width(line)) / 2
		if gap < 0 {
			gap = 0
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Repeat(" ", gap) + line)
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// pill renders a small label badge with given foreground and background colors.
func pill(label string, fg, bg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(label)
}

const maxContentWidth = 100

func contentWidth(termWidth int) int {
	if termWidth < maxContentWidth {
		return termWidth
	}
	return maxContentWidth
}

func centerBlock(s string, termWidth int) string {
	cw := contentWidth(termWidth)
	margin := (termWidth - cw) / 2
	if margin <= 0 {
		return s
	}
	pad := strings.Repeat(" ", margin)
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = pad + ln
	}
	return strings.Join(lines, "\n")
}
