package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogViewer displays the last N lines of ~/.seshr/debug.log in a viewport.
type LogViewer struct {
	vp     viewport.Model
	width  int
	height int
	ready  bool
}

// NewLogViewer constructs a LogViewer and loads the debug log.
func NewLogViewer(width, height int) LogViewer {
	lv := LogViewer{width: width, height: height}
	lv.vp = viewport.New(width, height-3)
	lv.vp.SetContent(lv.loadLog())
	lv.vp.GotoBottom()
	lv.ready = true
	return lv
}

// SetSize updates dimensions and reconfigures the viewport.
func (lv LogViewer) SetSize(width, height int) LogViewer {
	lv.width = width
	lv.height = height
	lv.vp.Width = width
	lv.vp.Height = height - 3
	return lv
}

// Update handles scroll keys. Returns done=true when user presses esc/q.
func (lv LogViewer) Update(msg tea.Msg) (LogViewer, bool) {
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "esc", "q":
			return lv, true
		case "g":
			lv.vp.GotoTop()
			return lv, false
		case "G":
			lv.vp.GotoBottom()
			return lv, false
		}
	}
	var cmd tea.Cmd
	lv.vp, cmd = lv.vp.Update(msg)
	_ = cmd
	return lv, false
}

// View renders the log viewer full-screen.
func (lv LogViewer) View() string {
	title := textStyle.Bold(true).Render("Debug Log") +
		"  " + dimStyle.Render("~/.seshr/debug.log")
	hint := dimStyle.Render("j/k scroll · g/G top/bottom · esc close")

	header := lipgloss.NewStyle().
		Width(lv.width).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colSurface1).
		Render(title)

	footer := lipgloss.NewStyle().
		Width(lv.width).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colSurface1).
		Render(hint)

	return header + "\n" + lv.vp.View() + "\n" + footer
}

// loadLog reads the last 1000 lines of the debug log. Returns a placeholder
// if the file does not exist.
func (lv LogViewer) loadLog() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return dimStyle.Render("(could not determine home directory)")
	}
	path := filepath.Join(home, ".seshr", "debug.log")
	data, err := os.ReadFile(path)
	if err != nil {
		return dimStyle.Render("(no log file found at " + path + ")")
	}
	lines := strings.Split(string(data), "\n")
	const maxLines = 1000
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
