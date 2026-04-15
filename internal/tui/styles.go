package tui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha (dark) / Latte (light) adaptive color palette.
var (
	colBase     = lipgloss.AdaptiveColor{Dark: "#1e1e2e", Light: "#eff1f5"}
	colMantle   = lipgloss.AdaptiveColor{Dark: "#181825", Light: "#e6e9ef"}
	colCrust    = lipgloss.AdaptiveColor{Dark: "#11111b", Light: "#dce0e8"}
	colSurface0 = lipgloss.AdaptiveColor{Dark: "#313244", Light: "#ccd0da"}
	colSurface1 = lipgloss.AdaptiveColor{Dark: "#45475a", Light: "#bcc0cc"}
	colSurface2 = lipgloss.AdaptiveColor{Dark: "#585b70", Light: "#acb0be"}
	colOverlay0 = lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#9ca0b0"}
	colOverlay1 = lipgloss.AdaptiveColor{Dark: "#7f849c", Light: "#8c8fa1"}
	colText     = lipgloss.AdaptiveColor{Dark: "#cdd6f4", Light: "#4c4f69"}
	colSubtext1 = lipgloss.AdaptiveColor{Dark: "#bac2de", Light: "#5c5f77"}
	colSubtext0 = lipgloss.AdaptiveColor{Dark: "#a6adc8", Light: "#6c6f85"}
	colRed      = lipgloss.AdaptiveColor{Dark: "#f38ba8", Light: "#d20f39"}
	colMaroon   = lipgloss.AdaptiveColor{Dark: "#eba0ac", Light: "#e64553"}
	colPeach    = lipgloss.AdaptiveColor{Dark: "#fab387", Light: "#fe640b"}
	colYellow   = lipgloss.AdaptiveColor{Dark: "#f9e2af", Light: "#df8e1d"}
	colGreen    = lipgloss.AdaptiveColor{Dark: "#a6e3a1", Light: "#40a02b"}
	colTeal     = lipgloss.AdaptiveColor{Dark: "#94e2d5", Light: "#179299"}
	colSky      = lipgloss.AdaptiveColor{Dark: "#89dceb", Light: "#04a5e5"}
	colBlue     = lipgloss.AdaptiveColor{Dark: "#89b4fa", Light: "#1e66f5"}
	colLavender = lipgloss.AdaptiveColor{Dark: "#b4befe", Light: "#7287fd"}
	colMauve    = lipgloss.AdaptiveColor{Dark: "#cba6f7", Light: "#8839ef"}
	colPink     = lipgloss.AdaptiveColor{Dark: "#f5c2e7", Light: "#ea76cb"}
	colFlamingo = lipgloss.AdaptiveColor{Dark: "#f2cdcd", Light: "#dd7878"}
)

// Semantic style variables used across the TUI.
var (
	subtitleStyle    = lipgloss.NewStyle().Foreground(colSubtext0)
	textStyle        = lipgloss.NewStyle().Foreground(colText)
	dimStyle         = lipgloss.NewStyle().Foreground(colOverlay0)
	subtleStyle      = lipgloss.NewStyle().Foreground(colSurface2)
	accentStyle      = lipgloss.NewStyle().Foreground(colPink).Bold(true)
	successStyle     = lipgloss.NewStyle().Foreground(colGreen)
	warningStyle     = lipgloss.NewStyle().Foreground(colYellow)
	errorStyle       = lipgloss.NewStyle().Foreground(colRed)
	selectedRowStyle = lipgloss.NewStyle().Background(colSurface0).Bold(true)
	keyStyle         = lipgloss.NewStyle().Foreground(colLavender).Bold(true)
	descStyle        = lipgloss.NewStyle().Foreground(colSubtext0)
	boxStyle         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colSurface1).Padding(0, 1)
	activeBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMauve).Padding(0, 1)
)

// Styles bundles the shared lipgloss styles derived from a Theme.
// Kept for backward compatibility with sessions.go and other consumers.
type Styles struct {
	App   lipgloss.Style
	Title lipgloss.Style
	Hint  lipgloss.Style
	Error lipgloss.Style
}

// NewStyles builds a Styles from a Theme.
func NewStyles(t Theme) Styles {
	return Styles{
		App:   lipgloss.NewStyle().Foreground(t.Foreground),
		Title: lipgloss.NewStyle().Foreground(t.Accent).Bold(true),
		Hint:  lipgloss.NewStyle().Foreground(t.Muted),
		Error: lipgloss.NewStyle().Foreground(t.Error),
	}
}
