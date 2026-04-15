package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette used across the TUI. All styles must reference
// a Theme field — no hardcoded colors elsewhere.
//
// TODO(phase-6): add Nord and Dracula variants; wire selection via settings.
type Theme struct {
	Name       string
	Background lipgloss.AdaptiveColor
	Foreground lipgloss.AdaptiveColor
	Accent     lipgloss.AdaptiveColor
	Muted      lipgloss.AdaptiveColor
	TokenBar   lipgloss.AdaptiveColor
	TokenEmpty lipgloss.AdaptiveColor
	Error      lipgloss.AdaptiveColor
}

// CatppuccinMocha is the default theme.
func CatppuccinMocha() Theme {
	return Theme{
		Name:       "catppuccin-mocha",
		Background: colBase,
		Foreground: colText,
		Accent:     colBlue,
		Muted:      colOverlay0,
		TokenBar:   colGreen,
		TokenEmpty: colSurface0,
		Error:      colRed,
	}
}
