package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg is emitted by the auto-play ticker each interval.
type TickMsg struct{}

// SpeedToDelay maps a 1..9 speed level to a tick delay (clamped).
// Level 1 = 2s (slowest), level 9 = 100ms (fastest), linear interpolation.
func SpeedToDelay(level int) time.Duration {
	if level < 1 {
		level = 1
	}
	if level > 9 {
		level = 9
	}
	const slow = 2000
	const fast = 100
	ms := slow - ((slow-fast)*(level-1))/8
	return time.Duration(ms) * time.Millisecond
}

// AutoPlayCmd returns a tea.Cmd that emits a TickMsg after delay.
func AutoPlayCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return TickMsg{} })
}
