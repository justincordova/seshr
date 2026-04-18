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

	// Palette entries used by replay styles.
	Overlay1 lipgloss.AdaptiveColor
	Subtext0 lipgloss.AdaptiveColor
	Surface0 lipgloss.AdaptiveColor

	// Role badge colors for replay message headers.
	UserColor       lipgloss.AdaptiveColor
	AssistantColor  lipgloss.AdaptiveColor
	ToolUseColor    lipgloss.AdaptiveColor
	ToolResultColor lipgloss.AdaptiveColor
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

		Overlay1: colOverlay1,
		Subtext0: colSubtext0,
		Surface0: colSurface0,

		UserColor:       lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"},
		AssistantColor:  lipgloss.AdaptiveColor{Light: "#1e66f5", Dark: "#89b4fa"},
		ToolUseColor:    lipgloss.AdaptiveColor{Light: "#df8e1d", Dark: "#f9e2af"},
		ToolResultColor: lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#6c7086"},
	}
}
