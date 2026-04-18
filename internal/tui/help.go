package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Help is an overlay that lists keybindings for the current screen.
// Any key dismisses it. Width/Height must be set before rendering.
type Help struct {
	bindings []key.Binding
	width    int
	height   int
}

// NewHelp constructs a Help overlay with the given bindings.
func NewHelp(bindings []key.Binding, width, height int) Help {
	return Help{bindings: bindings, width: width, height: height}
}

// SetSize updates dimensions.
func (h Help) SetSize(width, height int) Help {
	h.width = width
	h.height = height
	return h
}

// View renders the centered keybinding modal.
func (h Help) View() string {
	var sb strings.Builder
	for _, b := range h.bindings {
		help := b.Help()
		if help.Key == "" && help.Desc == "" {
			continue
		}
		sb.WriteString(keyStyle.Render(help.Key))
		sb.WriteString("  ")
		sb.WriteString(descStyle.Render(help.Desc))
		sb.WriteString("\n")
	}

	// Global bindings always shown.
	globals := []struct{ k, d string }{
		{"?", "help"},
		{"/", "search"},
		{",", "settings"},
		{"L", "log viewer"},
	}
	if sb.Len() > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("── global ──") + "\n")
	}
	for _, g := range globals {
		sb.WriteString(keyStyle.Render(g.k))
		sb.WriteString("  ")
		sb.WriteString(descStyle.Render(g.d))
		sb.WriteString("\n")
	}

	content := strings.TrimRight(sb.String(), "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colMauve).
		Padding(1, 2).
		Render(
			textStyle.Render("Keybindings") + "\n\n" +
				content + "\n\n" +
				dimStyle.Render("press any key to close"),
		)

	if h.width <= 0 || h.height <= 0 {
		return box
	}
	return lipgloss.Place(h.width, h.height, lipgloss.Center, lipgloss.Center, box)
}
