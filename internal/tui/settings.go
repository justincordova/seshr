package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justincordova/seshly/internal/config"
)

// SettingsSavedMsg is emitted when the user saves settings.
type SettingsSavedMsg struct {
	Cfg config.Config
}

// fieldID identifies an editable settings field.
type fieldID int

const (
	fieldTheme fieldID = iota
	fieldGap
	fieldContext
	fieldCount // sentinel
)

var fieldLabels = [fieldCount]string{
	fieldTheme:   "Theme",
	fieldGap:     "Gap threshold (seconds)",
	fieldContext: "Default context window (tokens)",
}

var availableThemes = []string{"catppuccin-mocha", "nord", "dracula"}

// Settings is the settings overlay model.
type Settings struct {
	cfg     config.Config
	cursor  fieldID
	editing bool
	input   textinput.Model
	width   int
	height  int
	err     string
}

// NewSettings constructs a Settings overlay from the live config.
func NewSettings(cfg config.Config, width, height int) Settings {
	ti := textinput.New()
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colLavender).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colText)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colLavender)
	ti.CharLimit = 64
	return Settings{cfg: cfg, input: ti, width: width, height: height}
}

// SetSize updates dimensions.
func (s Settings) SetSize(width, height int) Settings {
	s.width = width
	s.height = height
	return s
}

// Update handles keys. Returns (updated Settings, close bool, cmd).
func (s Settings) Update(msg tea.Msg) (Settings, bool, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		if s.editing {
			s.input, _ = s.input.Update(msg)
		}
		return s, false, nil
	}

	if s.editing {
		switch km.String() {
		case "esc":
			s.editing = false
			s.input.Blur()
			s.err = ""
			return s, false, nil
		case "enter":
			if err := s.applyInput(); err != nil {
				s.err = err.Error()
			} else {
				s.editing = false
				s.input.Blur()
				s.err = ""
			}
			return s, false, nil
		default:
			s.input, _ = s.input.Update(msg)
			return s, false, nil
		}
	}

	switch km.String() {
	case "esc", "q":
		return s, true, nil
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < fieldCount-1 {
			s.cursor++
		}
	case "enter", " ":
		if s.cursor == fieldTheme {
			// Cycle through themes without a text input.
			s.cfg.Theme = nextTheme(s.cfg.Theme)
			return s, false, nil
		}
		s.startEditing()
		return s, false, nil
	case "s":
		if err := config.Save(s.cfg); err != nil {
			s.err = fmt.Sprintf("save: %v", err)
		} else {
			cmd := func() tea.Msg { return SettingsSavedMsg{Cfg: s.cfg} }
			return s, true, cmd
		}
	}
	return s, false, nil
}

func (s *Settings) startEditing() {
	s.input.SetValue(s.fieldValue(s.cursor))
	s.input.Focus()
	s.input.CursorEnd()
	s.editing = true
}

func (s *Settings) applyInput() error {
	val := strings.TrimSpace(s.input.Value())
	switch s.cursor {
	case fieldGap:
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil || n < 0 {
			return fmt.Errorf("must be a non-negative integer")
		}
		s.cfg.GapThresholdSeconds = n
	case fieldContext:
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil || n <= 0 {
			return fmt.Errorf("must be a positive integer")
		}
		s.cfg.DefaultContextWindow = n
	}
	return nil
}

func (s Settings) fieldValue(f fieldID) string {
	switch f {
	case fieldTheme:
		return s.cfg.Theme
	case fieldGap:
		return fmt.Sprintf("%d", s.cfg.GapThresholdSeconds)
	case fieldContext:
		return fmt.Sprintf("%d", s.cfg.DefaultContextWindow)
	}
	return ""
}

func nextTheme(current string) string {
	for i, t := range availableThemes {
		if t == current {
			return availableThemes[(i+1)%len(availableThemes)]
		}
	}
	return availableThemes[0]
}

// View renders the settings panel centered on screen.
func (s Settings) View() string {
	var sb strings.Builder
	sb.WriteString(textStyle.Bold(true).Render("Settings") + "\n\n")

	for i := fieldID(0); i < fieldCount; i++ {
		label := fieldLabels[i]
		val := s.fieldValue(i)

		var line string
		if s.cursor == i {
			if s.editing && i != fieldTheme {
				line = keyStyle.Render("▸ "+label+": ") + s.input.View()
			} else {
				line = keyStyle.Render("▸ "+label+": ") + textStyle.Render(val)
			}
		} else {
			line = dimStyle.Render("  "+label+": ") + subtitleStyle.Render(val)
		}
		sb.WriteString(line + "\n")
	}

	if s.err != "" {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(colRed).Render(s.err) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ navigate · enter edit · space cycle theme · s save · esc close"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colMauve).
		Padding(1, 2).
		Width(54).
		Render(sb.String())

	if s.width <= 0 || s.height <= 0 {
		return box
	}
	return lipgloss.Place(s.width, s.height, lipgloss.Center, lipgloss.Center, box)
}
