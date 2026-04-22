package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tui/clipboard"
)

// ResumeOverlayModel renders the "Resume this session" overlay.
type ResumeOverlayModel struct {
	kind      session.SourceKind
	sessionID string
	copiedAt  time.Time
	copyErr   error
	width     int
	height    int
	th        Theme
}

// NewResumeOverlay returns a resume overlay for the given session.
func NewResumeOverlay(kind session.SourceKind, id string, th Theme) ResumeOverlayModel {
	return ResumeOverlayModel{kind: kind, sessionID: id, th: th}
}

func (m ResumeOverlayModel) Init() tea.Cmd { return nil }

func (m ResumeOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return CloseResumeOverlayMsg{} }
		case "enter":
			cmd := m.resumeCommand()
			err := clipboard.Copy(cmd)
			m.copyErr = err
			if err == nil {
				m.copiedAt = time.Now()
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m ResumeOverlayModel) View() string {
	cmd := m.resumeCommand()

	title := lipgloss.NewStyle().Bold(true).Render("Resume this session")
	cmdLine := lipgloss.NewStyle().Foreground(m.th.Accent).Bold(true).Render(cmd)

	var footer string
	switch {
	case m.copyErr != nil:
		footer = dimStyle.Render("copy unavailable · select and copy manually")
	case !m.copiedAt.IsZero() && time.Since(m.copiedAt) < 2*time.Second:
		footer = lipgloss.NewStyle().Foreground(m.th.Success).Render("✓ copied — paste in your terminal")
	default:
		footer = dimStyle.Render("enter  copy to clipboard    esc  close")
	}

	content := strings.Join([]string{
		title,
		"",
		cmdLine,
		"",
		footer,
	}, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.th.Accent).
		Padding(1, 2).
		Width(min(60, m.width-4)).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// resumeCommand returns the source-appropriate resume command.
func (m ResumeOverlayModel) resumeCommand() string {
	switch m.kind {
	case session.SourceClaude:
		return fmt.Sprintf("claude --resume %s", m.sessionID)
	case session.SourceOpenCode:
		return fmt.Sprintf("opencode -s %s", m.sessionID)
	default:
		return fmt.Sprintf("# resume %s (unknown source)", m.sessionID)
	}
}

// CloseResumeOverlayMsg is emitted when the overlay is dismissed.
type CloseResumeOverlayMsg struct{}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
