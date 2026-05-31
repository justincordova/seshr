package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg is emitted by the auto-play ticker each interval. Gen tags the
// ticker generation that scheduled it; tea.Tick cannot be cancelled, so a
// stale tick from a paused/superseded chain carries an old Gen and is
// ignored by the handler. Without this, pause→resume or a speed change while
// playing would fork additional self-perpetuating tick chains.
type TickMsg struct{ Gen int }

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

// AutoPlayCmd returns a tea.Cmd that emits a TickMsg after delay, tagged
// with the given generation so the handler can reject stale ticks.
func AutoPlayCmd(delay time.Duration, gen int) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg { return TickMsg{Gen: gen} })
}
