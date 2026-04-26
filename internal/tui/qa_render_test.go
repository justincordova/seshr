package tui

// QA rendering harness — these are human-eyeball tests. Run with:
//
//   go test ./internal/tui/ -run TestQA -v
//
// They render the picker into stdout (via t.Log) so a developer can scan
// the output for layout regressions. Skipped in normal CI by checking for
// the SESHR_QA env var.

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/session"
)

func qaSkip(t *testing.T) {
	if os.Getenv("SESHR_QA") == "" {
		t.Skip("set SESHR_QA=1 to run QA rendering tests")
	}
}

func qaMetas() []backend.SessionMeta {
	now := time.Now()
	return []backend.SessionMeta{
		{ID: "bb859dee-0744-44f1-8654-0f93eeede8e4", Project: "-Users-justin-cs-projects-seshr", Directory: "/Users/justin/cs/projects/seshr", Kind: session.SourceClaude, TokenCount: 15_700_000, UpdatedAt: now.Add(-2 * time.Minute)},
		{ID: "23afb0c1d2e3f4", Project: "dotfiles", Directory: "/Users/justin/cs/projects/dotfiles", Kind: session.SourceOpenCode, TokenCount: 8_200_000, UpdatedAt: now.Add(-30 * time.Second)},
		{ID: "a91c4d8eef0011", Project: "-Users-justin-cs-projects-seshr", Directory: "/Users/justin/cs/projects/seshr", Kind: session.SourceClaude, TokenCount: 4_100_000, UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "77eb3199aabbcc", Project: "web-app", Directory: "/Users/justin/cs/projects/web-app", Kind: session.SourceOpenCode, TokenCount: 912_000, UpdatedAt: now.Add(-30 * time.Hour)},
		{ID: "5c151351-9566-49f8-9752-4bef33b34859", Project: "-Users-justin-cs-njit-portfolio", Directory: "/Users/justin/cs/njit/portfolio", Kind: session.SourceClaude, TokenCount: 1_200_000, UpdatedAt: now.Add(-3 * 24 * time.Hour)},
		{ID: "d29d5ec6-e19d-4362-bd67-7582a6b95ded", Project: "-Users-justin-cs-njit-dartly", Directory: "/Users/justin/cs/njit/dartly", Kind: session.SourceClaude, TokenCount: 2_300_000, UpdatedAt: now.Add(-7 * 24 * time.Hour)},
	}
}

func qaLive() map[string]*backend.LiveSession {
	return map[string]*backend.LiveSession{
		"bb859dee-0744-44f1-8654-0f93eeede8e4": {
			SessionID:   "bb859dee-0744-44f1-8654-0f93eeede8e4",
			Status:      backend.StatusWorking,
			CurrentTask: "fixing failing tests in topics package",
		},
		"23afb0c1d2e3f4": {
			SessionID: "23afb0c1d2e3f4",
			Status:    backend.StatusWaiting,
		},
	}
}

func TestQA_RecentView_FullWidth(t *testing.T) {
	qaSkip(t)
	p := NewPicker(qaMetas(), CatppuccinMocha(), nil, config.PickerViewRecent)
	p.SetLiveIndex(qaLive())
	next, _ := p.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	p = next.(Picker)
	t.Logf("\n%s", p.View())
}

func TestQA_RecentView_NoLive(t *testing.T) {
	qaSkip(t)
	p := NewPicker(qaMetas(), CatppuccinMocha(), nil, config.PickerViewRecent)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	p = next.(Picker)
	t.Logf("\n%s", p.View())
}

func TestQA_ProjectView_WithLive(t *testing.T) {
	qaSkip(t)
	p := NewPicker(qaMetas(), CatppuccinMocha(), nil, config.PickerViewProject)
	p.SetLiveIndex(qaLive())
	next, _ := p.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	p = next.(Picker)
	t.Logf("\n%s", p.View())
}

func TestQA_ProjectView_NoLive(t *testing.T) {
	qaSkip(t)
	p := NewPicker(qaMetas(), CatppuccinMocha(), nil, config.PickerViewProject)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	p = next.(Picker)
	t.Logf("\n%s", p.View())
}

func TestQA_RecentView_NarrowWidth(t *testing.T) {
	qaSkip(t)
	p := NewPicker(qaMetas(), CatppuccinMocha(), nil, config.PickerViewRecent)
	p.SetLiveIndex(qaLive())
	next, _ := p.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	p = next.(Picker)
	t.Logf("\n%s", p.View())
}
