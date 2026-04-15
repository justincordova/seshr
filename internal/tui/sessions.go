package tui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dustin/go-humanize"
	"github.com/justincordova/agentlens/internal/parser"
)

// Picker is the Session Picker Bubbletea model. See SPEC §3.1.
type Picker struct {
	metas     []parser.SessionMeta
	cursor    int
	width     int
	height    int
	keys      PickerKeys
	styles    Styles
	confirm   *Confirm
	deleteErr error
}

// NewPicker builds a Picker from pre-scanned metadata.
func NewPicker(metas []parser.SessionMeta) Picker {
	return Picker{
		metas:  metas,
		keys:   DefaultPickerKeys(),
		styles: NewStyles(CatppuccinMocha()),
	}
}

// Cursor returns the current selection index.
func (p Picker) Cursor() int { return p.cursor }

// InConfirm reports whether a confirm modal is currently shown.
func (p Picker) InConfirm() bool { return p.confirm != nil }

// Metas returns the current session list (post-delete).
func (p Picker) Metas() []parser.SessionMeta { return p.metas }

// Selected returns the currently highlighted SessionMeta, or the zero value
// when the list is empty.
func (p Picker) Selected() (parser.SessionMeta, bool) {
	if len(p.metas) == 0 {
		return parser.SessionMeta{}, false
	}
	return p.metas[p.cursor], true
}

// Init satisfies tea.Model.
func (p Picker) Init() tea.Cmd { return nil }

// Update satisfies tea.Model.
func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.confirm != nil {
		if km, ok := msg.(tea.KeyMsg); ok {
			m, _ := p.confirm.Update(km)
			c := m.(Confirm)
			p.confirm = &c
			if c.Done() {
				if c.Confirmed() {
					p.deleteErr = deleteSelected(&p)
				}
				p.confirm = nil
			}
			return p, nil
		}
		return p, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, p.keys.Quit):
			return p, tea.Quit
		case key.Matches(msg, p.keys.Up):
			if p.cursor > 0 {
				p.cursor--
			}
			return p, nil
		case key.Matches(msg, p.keys.Down):
			if p.cursor < len(p.metas)-1 {
				p.cursor++
			}
			return p, nil
		case key.Matches(msg, p.keys.Open):
			return p, nil
		case key.Matches(msg, p.keys.Delete):
			if len(p.metas) == 0 {
				return p, nil
			}
			sel := p.metas[p.cursor]
			c := NewConfirm(
				"Delete session?",
				fmt.Sprintf("%s · %s\nThis cannot be undone.", sel.Project, sel.ID),
			)
			p.confirm = &c
			return p, nil
		}
	}
	return p, nil
}

// View satisfies tea.Model.
func (p Picker) View() string {
	if p.confirm != nil {
		return p.confirm.View()
	}
	if len(p.metas) == 0 {
		return p.styles.App.Render(
			p.styles.Title.Render("AgentLens") + "\n\n" +
				p.styles.Hint.Render("No sessions found in ~/.claude/projects/") + "\n\n" +
				p.styles.Hint.Render("press q to quit") + "\n",
		)
	}

	var b strings.Builder
	b.WriteString(p.styles.Title.Render(fmt.Sprintf("Sessions (%d found)", len(p.metas))))
	b.WriteString("\n\n")

	for i, m := range p.metas {
		marker := "  "
		if i == p.cursor {
			marker = "▸ "
		}
		line1 := fmt.Sprintf("%s%-32s  %s  %s",
			marker,
			Truncate(m.Project, 32),
			humanizeSize(m.Size),
			humanize.Time(m.ModifiedAt),
		)
		line2 := fmt.Sprintf("    %s · %s", m.Source, backupIndicator(m.HasBackup))
		if i == p.cursor {
			b.WriteString(p.styles.Title.Render(line1))
		} else {
			b.WriteString(line1)
		}
		b.WriteString("\n")
		b.WriteString(p.styles.Hint.Render(line2))
		b.WriteString("\n\n")
	}

	b.WriteString(p.styles.Hint.Render("j/k navigate · enter open · d delete · q quit"))
	b.WriteString("\n")
	return p.styles.App.Render(b.String())
}

// deleteSelected removes the currently-highlighted session's file and
// cleans up its parent directory if it becomes empty.
func deleteSelected(p *Picker) error {
	if len(p.metas) == 0 {
		return nil
	}
	sel := p.metas[p.cursor]
	if err := os.Remove(sel.Path); err != nil {
		slog.Error("delete session failed", "path", sel.Path, "err", err)
		return err
	}
	slog.Info("deleted session", "path", sel.Path)
	dir := filepath.Dir(sel.Path)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		if err := os.Remove(dir); err == nil {
			slog.Info("removed empty project dir", "path", dir)
		}
	}
	p.metas = append(p.metas[:p.cursor], p.metas[p.cursor+1:]...)
	if p.cursor >= len(p.metas) && p.cursor > 0 {
		p.cursor--
	}
	return nil
}

func humanizeSize(n int64) string {
	return humanize.IBytes(uint64(n))
}

func backupIndicator(has bool) string {
	if has {
		return "↶ has backup"
	}
	return ""
}
