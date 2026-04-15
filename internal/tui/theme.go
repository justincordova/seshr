package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette used across the TUI. All styles must reference
// a Theme field — no hardcoded colors elsewhere.
//
// TODO(phase-6): add Nord and Dracula variants; wire selection via settings.
type Theme struct {
	Name       string
	Background lipgloss.Color
	Foreground lipgloss.Color
	Accent     lipgloss.Color
	Muted      lipgloss.Color
	TokenBar   lipgloss.Color
	TokenEmpty lipgloss.Color
	Error      lipgloss.Color
}

// CatppuccinMocha is the default theme.
func CatppuccinMocha() Theme {
	return Theme{
		Name:       "catppuccin-mocha",
		Background: lipgloss.Color("#1e1e2e"),
		Foreground: lipgloss.Color("#cdd6f4"),
		Accent:     lipgloss.Color("#89b4fa"),
		Muted:      lipgloss.Color("#6c7086"),
		TokenBar:   lipgloss.Color("#a6e3a1"),
		TokenEmpty: lipgloss.Color("#313244"),
		Error:      lipgloss.Color("#f38ba8"),
	}
}
