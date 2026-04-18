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
