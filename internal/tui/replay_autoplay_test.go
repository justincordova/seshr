package tui_test

import (
	"testing"
	"time"

	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestSpeedToDelay_BoundsAndMonotonic(t *testing.T) {
	slow := tui.SpeedToDelay(1)
	fast := tui.SpeedToDelay(9)

	assert.Equal(t, 2*time.Second, slow)
	assert.Equal(t, 100*time.Millisecond, fast)
	assert.True(t, tui.SpeedToDelay(1) > tui.SpeedToDelay(9), "slower level should be longer delay")
}

func TestSpeedToDelay_OutOfRangeClamped(t *testing.T) {
	assert.Equal(t, tui.SpeedToDelay(1), tui.SpeedToDelay(0))
	assert.Equal(t, tui.SpeedToDelay(9), tui.SpeedToDelay(99))
}

func TestAutoPlayCmd_ReturnsNonNil(t *testing.T) {
	cmd := tui.AutoPlayCmd(100 * time.Millisecond)
	assert.NotNil(t, cmd, "AutoPlayCmd must return a non-nil tea.Cmd")
}
