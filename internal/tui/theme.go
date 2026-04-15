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
		Background: lipgloss.AdaptiveColor{Dark: "#1e1e2e", Light: "#eff1f5"},
		Foreground: lipgloss.AdaptiveColor{Dark: "#cdd6f4", Light: "#4c4f69"},
		Accent:     lipgloss.AdaptiveColor{Dark: "#89b4fa", Light: "#1e66f5"},
		Muted:      lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#9ca0b0"},
		TokenBar:   lipgloss.AdaptiveColor{Dark: "#a6e3a1", Light: "#40a02b"},
		TokenEmpty: lipgloss.AdaptiveColor{Dark: "#313244", Light: "#ccd0da"},
		Error:      lipgloss.AdaptiveColor{Dark: "#f38ba8", Light: "#d20f39"},
	}
}
