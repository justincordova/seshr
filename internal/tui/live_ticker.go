package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// liveSlowMsg is the slow-tick message (10s): run detectors, reconcile index.
type liveSlowMsg struct{ At time.Time }

// liveFastMsg is the fast-tick message (2s): incremental load for live sessions.
type liveFastMsg struct{ At time.Time }

func slowTickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return liveSlowMsg{At: t} })
}

func fastTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return liveFastMsg{At: t} })
}

// ShouldRunFastTick returns true when there are live sessions and no overlay
// is blocking the ticker. Exported for testing.
func ShouldRunFastTick(liveCount int, overlayOpen bool) bool {
	return liveCount > 0 && !overlayOpen
}

// shouldRunFastTick is the unexported alias for internal use.
func shouldRunFastTick(liveCount int, overlayOpen bool) bool {
	return ShouldRunFastTick(liveCount, overlayOpen)
}
