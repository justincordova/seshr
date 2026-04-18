package tui_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestCatppuccinMocha_HasRoleBadgeColors(t *testing.T) {
	theme := tui.CatppuccinMocha()

	assert.NotEmpty(t, theme.UserColor.Dark, "UserColor.Dark must be set")
	assert.NotEmpty(t, theme.UserColor.Light, "UserColor.Light must be set")
	assert.NotEmpty(t, theme.AssistantColor.Dark)
	assert.NotEmpty(t, theme.AssistantColor.Light)
	assert.NotEmpty(t, theme.ToolUseColor.Dark)
	assert.NotEmpty(t, theme.ToolUseColor.Light)
	assert.NotEmpty(t, theme.ToolResultColor.Dark)
	assert.NotEmpty(t, theme.ToolResultColor.Light)
}

func TestNord_HasRequiredFields(t *testing.T) {
	th := tui.Nord()
	assert.Equal(t, "nord", th.Name)
	assert.NotEmpty(t, th.Background.Dark)
	assert.NotEmpty(t, th.Foreground.Dark)
	assert.NotEmpty(t, th.Accent.Dark)
	assert.NotEmpty(t, th.Error.Dark)
	assert.NotEmpty(t, th.UserColor.Dark)
	assert.NotEmpty(t, th.AssistantColor.Dark)
}

func TestDracula_HasRequiredFields(t *testing.T) {
	th := tui.Dracula()
	assert.Equal(t, "dracula", th.Name)
	assert.NotEmpty(t, th.Background.Dark)
	assert.NotEmpty(t, th.Foreground.Dark)
	assert.NotEmpty(t, th.Accent.Dark)
	assert.NotEmpty(t, th.Error.Dark)
	assert.NotEmpty(t, th.UserColor.Dark)
	assert.NotEmpty(t, th.AssistantColor.Dark)
}

func TestThemeByName_ReturnsCorrectTheme(t *testing.T) {
	cases := []struct {
		input string
		name  string
	}{
		{"nord", "nord"},
		{"dracula", "dracula"},
		{"catppuccin-mocha", "catppuccin-mocha"},
		{"unknown", "catppuccin-mocha"},
		{"", "catppuccin-mocha"},
	}
	for _, tc := range cases {
		th := tui.ThemeByName(tc.input)
		assert.Equal(t, tc.name, th.Name, "input=%q", tc.input)
	}
}

func TestNewStyles_ProducesValidStylesForAllThemes(t *testing.T) {
	themes := []tui.Theme{tui.CatppuccinMocha(), tui.Nord(), tui.Dracula()}
	for _, th := range themes {
		s := tui.NewStyles(th)
		assert.NotEmpty(t, s.App.Render("x"), "theme %s App style must render", th.Name)
		assert.NotEmpty(t, s.Title.Render("x"), "theme %s Title style must render", th.Name)
		assert.NotEmpty(t, s.Error.Render("x"), "theme %s Error style must render", th.Name)
	}
}

func TestAllThemes_HaveProjectPalette(t *testing.T) {
	themes := []tui.Theme{tui.CatppuccinMocha(), tui.Nord(), tui.Dracula()}
	for _, th := range themes {
		assert.GreaterOrEqual(t, len(th.ProjectPalette), 6, "theme %s should have >= 6 palette entries", th.Name)
	}
}
