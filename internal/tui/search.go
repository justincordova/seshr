package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type SearchBar struct {
	input    textinput.Model
	active   bool
	matches  []fuzzy.Match
	query    string
	matchIdx int
}

func NewSearchBar() SearchBar {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colLavender).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colText)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colLavender)
	ti.CharLimit = 200
	return SearchBar{
		input: ti,
	}
}

func (s *SearchBar) Active() bool           { return s.active }
func (s *SearchBar) Query() string          { return s.query }
func (s *SearchBar) MatchCount() int        { return len(s.matches) }
func (s *SearchBar) MatchIndex() int        { return s.matchIdx }
func (s *SearchBar) Matches() []fuzzy.Match { return s.matches }

func (s *SearchBar) Open() {
	s.active = true
	s.input.Focus()
	s.input.SetValue("")
	s.query = ""
	s.matches = nil
	s.matchIdx = 0
}

func (s *SearchBar) Close() {
	s.active = false
	s.input.Blur()
	s.input.SetValue("")
	s.query = ""
	s.matches = nil
	s.matchIdx = 0
}

func (s *SearchBar) Commit() {
	s.active = false
	s.input.Blur()
	s.query = s.input.Value()
	s.matchIdx = 0
}

func (s *SearchBar) NextMatch() (int, bool) {
	if len(s.matches) == 0 {
		return -1, false
	}
	s.matchIdx++
	if s.matchIdx >= len(s.matches) {
		s.matchIdx = 0
	}
	return s.matches[s.matchIdx].Index, true
}

func (s *SearchBar) PrevMatch() (int, bool) {
	if len(s.matches) == 0 {
		return -1, false
	}
	s.matchIdx--
	if s.matchIdx < 0 {
		s.matchIdx = len(s.matches) - 1
	}
	return s.matches[s.matchIdx].Index, true
}

func (s *SearchBar) CurrentMatch() (int, bool) {
	if len(s.matches) == 0 {
		return -1, false
	}
	return s.matches[s.matchIdx].Index, true
}

func (s *SearchBar) Filter(haystack []string) {
	if s.query == "" {
		s.matches = nil
		s.matchIdx = 0
		return
	}
	s.matches = fuzzy.Find(s.query, haystack)
	s.matchIdx = 0
}

func (s *SearchBar) Update(msg tea.Msg) {
	s.input, _ = s.input.Update(msg)
	q := s.input.Value()
	if q != s.query {
		s.query = q
	}
}

func (s SearchBar) View(width int) string {
	if !s.active {
		return ""
	}

	bar := s.input.View()

	count := ""
	if s.query != "" && len(s.matches) > 0 {
		count = dimStyle.Render(lipgloss.NewStyle().
			Foreground(colSubtext0).
			Render(fmt.Sprintf(" %d/%d", s.matchIdx+1, len(s.matches))))
	} else if s.query != "" {
		count = dimStyle.Render(" no match")
	}

	gap := width - lipgloss.Width(bar) - lipgloss.Width(count) - 2
	if gap < 0 {
		gap = 0
	}
	return lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		Background(colMantle).
		Render(bar + strings.Repeat(" ", gap) + count)
}

func searchKeyBinding() key.Binding {
	return key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
}
