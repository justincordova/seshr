package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds the color palette used across the TUI. All styles must reference
// a Theme field — no hardcoded colors elsewhere.
type Theme struct {
	Name       string
	Background lipgloss.AdaptiveColor
	Foreground lipgloss.AdaptiveColor
	Accent     lipgloss.AdaptiveColor
	Muted      lipgloss.AdaptiveColor
	Error      lipgloss.AdaptiveColor

	// Palette entries used by replay styles.
	Overlay1 lipgloss.AdaptiveColor
	Subtext0 lipgloss.AdaptiveColor
	Surface0 lipgloss.AdaptiveColor

	ProjectPalette []lipgloss.AdaptiveColor

	// Role badge colors for replay message headers.
	UserColor       lipgloss.AdaptiveColor
	AssistantColor  lipgloss.AdaptiveColor
	ToolUseColor    lipgloss.AdaptiveColor
	ToolResultColor lipgloss.AdaptiveColor
	AgentColor      lipgloss.AdaptiveColor
}

// ThemeByName returns the Theme matching name (case-insensitive). Falls back
// to CatppuccinMocha for unknown names.
func ThemeByName(name string) Theme {
	switch name {
	case "nord":
		return Nord()
	case "dracula":
		return Dracula()
	default:
		return CatppuccinMocha()
	}
}

// CatppuccinMocha is the default theme.
func CatppuccinMocha() Theme {
	return Theme{
		Name:       "catppuccin-mocha",
		Background: colBase,
		Foreground: colText,
		Accent:     colBlue,
		Muted:      colOverlay0,
		Error:      colRed,

		Overlay1: colOverlay1,
		Subtext0: colSubtext0,
		Surface0: colSurface0,

		ProjectPalette: []lipgloss.AdaptiveColor{
			colMauve, colBlue, colGreen, colLavender, colPink, colFlamingo,
			{Dark: "#fab387", Light: "#fe640b"},
			{Dark: "#94e2d5", Light: "#179299"},
		},

		UserColor:       lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"},
		AssistantColor:  lipgloss.AdaptiveColor{Light: "#1e66f5", Dark: "#89b4fa"},
		ToolUseColor:    lipgloss.AdaptiveColor{Light: "#df8e1d", Dark: "#f9e2af"},
		ToolResultColor: lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#6c7086"},
		AgentColor:      lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#cba6f7"},
	}
}

// Nord theme — Polar Night / Snow Storm / Aurora palette.
func Nord() Theme {
	bg := lipgloss.AdaptiveColor{Dark: "#2E3440", Light: "#ECEFF4"}
	fg := lipgloss.AdaptiveColor{Dark: "#D8DEE9", Light: "#2E3440"}
	accent := lipgloss.AdaptiveColor{Dark: "#88C0D0", Light: "#5E81AC"}
	muted := lipgloss.AdaptiveColor{Dark: "#4C566A", Light: "#9EACBA"}
	surface1 := lipgloss.AdaptiveColor{Dark: "#434C5E", Light: "#D8DEE9"}
	overlay1 := lipgloss.AdaptiveColor{Dark: "#616E88", Light: "#7B88A1"}
	subtext := lipgloss.AdaptiveColor{Dark: "#A0AABB", Light: "#4C566A"}
	errCol := lipgloss.AdaptiveColor{Dark: "#BF616A", Light: "#BF616A"}
	frost2 := lipgloss.AdaptiveColor{Dark: "#81A1C1", Light: "#81A1C1"}
	frost3 := lipgloss.AdaptiveColor{Dark: "#5E81AC", Light: "#5E81AC"}
	auroraGreen := lipgloss.AdaptiveColor{Dark: "#A3BE8C", Light: "#A3BE8C"}
	auroraYellow := lipgloss.AdaptiveColor{Dark: "#EBCB8B", Light: "#EBCB8B"}
	auroraPurple := lipgloss.AdaptiveColor{Dark: "#B48EAD", Light: "#B48EAD"}
	auroraRed := lipgloss.AdaptiveColor{Dark: "#BF616A", Light: "#D08770"}

	return Theme{
		Name:       "nord",
		Background: bg,
		Foreground: fg,
		Accent:     accent,
		Muted:      muted,
		Error:      errCol,

		Overlay1: overlay1,
		Subtext0: subtext,
		Surface0: surface1,

		ProjectPalette: []lipgloss.AdaptiveColor{
			frost3, frost2, accent, auroraGreen, auroraYellow, auroraPurple,
			auroraRed, {Dark: "#88C0D0", Light: "#88C0D0"},
		},

		UserColor:       lipgloss.AdaptiveColor{Light: "#A3BE8C", Dark: "#A3BE8C"},
		AssistantColor:  lipgloss.AdaptiveColor{Light: "#5E81AC", Dark: "#88C0D0"},
		ToolUseColor:    lipgloss.AdaptiveColor{Light: "#EBCB8B", Dark: "#EBCB8B"},
		ToolResultColor: lipgloss.AdaptiveColor{Light: "#7B88A1", Dark: "#616E88"},
		AgentColor:      lipgloss.AdaptiveColor{Light: "#B48EAD", Dark: "#B48EAD"},
	}
}

// Dracula theme — dark background, vivid accent palette.
func Dracula() Theme {
	bg := lipgloss.AdaptiveColor{Dark: "#282A36", Light: "#F8F8F2"}
	fg := lipgloss.AdaptiveColor{Dark: "#F8F8F2", Light: "#282A36"}
	accent := lipgloss.AdaptiveColor{Dark: "#8BE9FD", Light: "#6272A4"}
	muted := lipgloss.AdaptiveColor{Dark: "#6272A4", Light: "#9AACCE"}
	surface1 := lipgloss.AdaptiveColor{Dark: "#383A59", Light: "#D8D8E8"}
	overlay1 := lipgloss.AdaptiveColor{Dark: "#BD93F9", Light: "#7B6FBF"}
	subtext := lipgloss.AdaptiveColor{Dark: "#BFBFCF", Light: "#44475A"}
	errCol := lipgloss.AdaptiveColor{Dark: "#FF5555", Light: "#FF5555"}
	return Theme{
		Name:       "dracula",
		Background: bg,
		Foreground: fg,
		Accent:     accent,
		Muted:      muted,
		Error:      errCol,

		Overlay1: overlay1,
		Subtext0: subtext,
		Surface0: surface1,

		ProjectPalette: []lipgloss.AdaptiveColor{
			{Dark: "#BD93F9", Light: "#BD93F9"},
			{Dark: "#FF79C6", Light: "#FF79C6"},
			{Dark: "#8BE9FD", Light: "#8BE9FD"},
			{Dark: "#50FA7B", Light: "#50FA7B"},
			{Dark: "#FFB86C", Light: "#FFB86C"},
			{Dark: "#F1FA8C", Light: "#F1FA8C"},
			{Dark: "#FF5555", Light: "#FF5555"},
			{Dark: "#6272A4", Light: "#6272A4"},
		},

		UserColor:       lipgloss.AdaptiveColor{Light: "#50FA7B", Dark: "#50FA7B"},
		AssistantColor:  lipgloss.AdaptiveColor{Light: "#6272A4", Dark: "#8BE9FD"},
		ToolUseColor:    lipgloss.AdaptiveColor{Light: "#FFB86C", Dark: "#FFB86C"},
		ToolResultColor: lipgloss.AdaptiveColor{Light: "#7B6FBF", Dark: "#6272A4"},
		AgentColor:      lipgloss.AdaptiveColor{Light: "#BD93F9", Dark: "#BD93F9"},
	}
}
