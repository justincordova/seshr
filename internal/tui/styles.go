package tui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha (dark) / Latte (light) adaptive color palette.
// Colors are added here as screens are built.
var (
	colBase     = lipgloss.AdaptiveColor{Dark: "#1e1e2e", Light: "#eff1f5"}
	colMantle   = lipgloss.AdaptiveColor{Dark: "#181825", Light: "#e6e9ef"}
	colSurface0 = lipgloss.AdaptiveColor{Dark: "#313244", Light: "#ccd0da"}
	colOverlay0 = lipgloss.AdaptiveColor{Dark: "#6c7086", Light: "#9ca0b0"}
	colText     = lipgloss.AdaptiveColor{Dark: "#cdd6f4", Light: "#4c4f69"}
	colSubtext0 = lipgloss.AdaptiveColor{Dark: "#a6adc8", Light: "#6c6f85"}
	colRed      = lipgloss.AdaptiveColor{Dark: "#f38ba8", Light: "#d20f39"}
	colGreen    = lipgloss.AdaptiveColor{Dark: "#a6e3a1", Light: "#40a02b"}
	colBlue     = lipgloss.AdaptiveColor{Dark: "#89b4fa", Light: "#1e66f5"}
	colLavender = lipgloss.AdaptiveColor{Dark: "#b4befe", Light: "#7287fd"}
	colMauve    = lipgloss.AdaptiveColor{Dark: "#cba6f7", Light: "#8839ef"}
)

// Semantic style variables used across the TUI.
var (
	subtitleStyle = lipgloss.NewStyle().Foreground(colSubtext0)
	dimStyle      = lipgloss.NewStyle().Foreground(colOverlay0)
	keyStyle      = lipgloss.NewStyle().Foreground(colLavender).Bold(true)
	descStyle     = lipgloss.NewStyle().Foreground(colSubtext0)
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
