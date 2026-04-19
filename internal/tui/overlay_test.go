package tui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Help overlay ──────────────────────────────────────────────────────────────

func TestApp_HelpOverlay_OpenOnQuestionMark(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	out := next.(tui.App).View()
	assert.Contains(t, out, "Keybindings", "help overlay must show 'Keybindings'")
}

func TestApp_HelpOverlay_AnyKeyDismisses(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	a1, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	require.Contains(t, a1.(tui.App).View(), "Keybindings")

	// Any key (e.g. 'x') should close the overlay.
	a2, _ := a1.(tui.App).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	out := a2.(tui.App).View()
	assert.NotContains(t, out, "Keybindings", "overlay should be dismissed")
}

func TestApp_HelpOverlay_ShowsGlobalBindings(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	out := next.(tui.App).View()
	assert.Contains(t, out, "?")
	assert.Contains(t, out, "settings")
	assert.Contains(t, out, "log viewer")
}

// ── Settings overlay ─────────────────────────────────────────────────────────

func TestApp_SettingsOverlay_OpenOnComma(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	out := next.(tui.App).View()
	assert.Contains(t, out, "Settings")
}

func TestApp_SettingsOverlay_EscCloses(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	a1, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	require.Contains(t, a1.(tui.App).View(), "Settings")

	a2, _ := a1.(tui.App).Update(tea.KeyMsg{Type: tea.KeyEsc})
	out := a2.(tui.App).View()
	assert.NotContains(t, out, "Settings")
}

func TestApp_SettingsOverlay_ShowsThemeField(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	out := next.(tui.App).View()
	assert.Contains(t, out, "Theme")
}

// ── Settings model unit tests ─────────────────────────────────────────────────

func TestSettings_CycleTheme(t *testing.T) {
	cfg := config.Default()
	cfg.Theme = "catppuccin-mocha"
	s := tui.NewSettings(cfg, 120, 40)

	// Press enter (on theme field, index 0) to cycle.
	s2, done, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	require.False(t, done)
	out := s2.View()
	assert.Contains(t, out, "nord", "should have cycled to nord")
}

func TestSettings_SaveEmitsMsg(t *testing.T) {
	cfg := config.Default()
	s := tui.NewSettings(cfg, 120, 40)

	_, done, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	require.True(t, done)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.SettingsSavedMsg)
	assert.True(t, ok, "cmd must emit SettingsSavedMsg")
}

func TestSettings_View_ContainsAllFields(t *testing.T) {
	s := tui.NewSettings(config.Default(), 120, 40)
	out := s.View()
	assert.Contains(t, out, "Theme")
	assert.Contains(t, out, "Gap threshold")
}

// ── Log viewer ────────────────────────────────────────────────────────────────

func TestApp_LogViewer_OpenOnL(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	next, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	out := next.(tui.App).View()
	assert.Contains(t, out, "Debug Log")
}

func TestApp_LogViewer_EscCloses(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	a1, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	require.Contains(t, a1.(tui.App).View(), "Debug Log")

	a2, _ := a1.(tui.App).Update(tea.KeyMsg{Type: tea.KeyEsc})
	out := a2.(tui.App).View()
	assert.NotContains(t, out, "Debug Log")
}

// ── Responsive: minimum terminal size ────────────────────────────────────────

func TestApp_TooSmall_ShowsWarning(t *testing.T) {
	app := tui.NewApp(nil, testCfg(), "")
	next, _ := app.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	out := next.(tui.App).View()
	assert.True(t, strings.Contains(out, "too small") || strings.Contains(out, "Terminal too small"),
		"should show too-small message, got: %s", out)
}

func TestApp_TooSmall_ClearsWhenAdequate(t *testing.T) {
	app := tui.NewApp(nil, testCfg(), "")
	a1, _ := app.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	require.Contains(t, a1.(tui.App).View(), "too small")

	a2, _ := a1.(tui.App).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	out := a2.(tui.App).View()
	assert.NotContains(t, out, "too small")
}

// ── Overlay resize ────────────────────────────────────────────────────────────

func TestApp_HelpOverlay_ResizenDoesNotPanic(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	a1, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	// Resize while overlay open — should not panic.
	a2, _ := a1.(tui.App).Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	out := a2.(tui.App).View()
	assert.Contains(t, out, "Keybindings")
}

func TestApp_SettingsOverlay_ResizenDoesNotPanic(t *testing.T) {
	app := newAppWithSize(t, 120, 40)
	a1, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	a2, _ := a1.(tui.App).Update(tea.WindowSizeMsg{Width: 80, Height: 25})
	out := a2.(tui.App).View()
	assert.Contains(t, out, "Settings")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newAppWithSize(t *testing.T, w, h int) tui.App {
	t.Helper()
	app := tui.NewApp(nil, testCfg(), "")
	next, _ := app.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(tui.App)
}
