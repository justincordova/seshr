package tui

import "github.com/charmbracelet/lipgloss"

// Styles bundles the shared lipgloss styles derived from a Theme.
//
// TODO(phase-4+): expand as screens are built.
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
