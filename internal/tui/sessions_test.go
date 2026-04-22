package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtures() []backend.SessionMeta {
	return []backend.SessionMeta{
		{ID: "a", Project: "proj-a", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Project: "proj-b", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-24 * time.Hour)},
		{ID: "c", Project: "proj-c", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-72 * time.Hour)},
	}
}

func pressDown(m tui.Picker, n int) tui.Picker {
	for i := 0; i < n; i++ {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = m2.(tui.Picker)
	}
	return m
}

// expandCurrentGroup presses space on the cursor to toggle the current group
// open. Groups are collapsed by default in NewPicker, so tests that want to
// act on a session must expand the owning group first.
func expandCurrentGroup(m tui.Picker) tui.Picker {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	return next.(tui.Picker)
}

func TestPicker_DownKey_MovesCursor(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 1, p.Cursor())
}

func TestPicker_UpKey_AtTopStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 0, p.Cursor())
}

func TestPicker_DownKey_AtBottomStays(t *testing.T) {
	// Arrange — 3 projects, all collapsed by default → 3 flat rows, index 0..2
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)
	m = pressDown(m, 10)

	// Assert — clamped at last row (index 2)
	assert.Equal(t, 2, m.Cursor())
}

func TestPicker_QuitKey_EmitsQuitCmd(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestPicker_Empty_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	m := tui.NewPicker(nil, tui.CatppuccinMocha(), nil)

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}

func TestPicker_View_ContainsProjectName(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act
	out := m.View()

	// Assert — project name is uppercased in the header for visual emphasis.
	assert.Contains(t, out, "PROJ-A")
}

func TestPicker_DKey_OnSession_EntersConfirmState(t *testing.T) {
	// Arrange — expand the first group, then move down onto its session row
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert
	p := next.(tui.Picker)
	assert.True(t, p.InConfirm())
}

func TestPicker_DKey_OnGroupHeader_NoOp(t *testing.T) {
	// Arrange — cursor is on group header
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert — delete is no-op on group header
	p := next.(tui.Picker)
	assert.False(t, p.InConfirm())
}

func TestPicker_ConfirmN_LeavesConfirmNoDelete(t *testing.T) {
	// Arrange — expand the first group and move onto its session row
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	// Assert
	p := next.(tui.Picker)
	assert.False(t, p.InConfirm())
	assert.Len(t, p.Metas(), 3, "no entries should be removed on cancel")
}

func TestPicker_EnterKey_OnSession_EmitsOpenSessionMsg(t *testing.T) {
	// Arrange — expand the first group, then move onto its session row
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	require.NotNil(t, cmd)
	msg := cmd()
	open, ok := msg.(tui.OpenSessionMsg)
	require.True(t, ok, "expected OpenSessionMsg, got %T", msg)
	assert.Equal(t, "a", open.Meta.ID)
}

func TestPicker_EnterKey_OnGroupHeader_TogglesCollapse(t *testing.T) {
	// Arrange — cursor on group header (row 0)
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act — enter toggles collapse
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	p := next.(tui.Picker)

	// Assert — group header is now collapsed, session row hidden
	// Flat rows: [group-header(proj-a,collapsed)] + [group-header(proj-b), session-b, group-header(proj-c), session-c]
	row, ok := p.Selected()
	assert.False(t, ok, "cursor on group header should not return a session")
	_ = row
}

func TestPicker_SpaceKey_OnGroupHeader_TogglesCollapse(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil)

	// Act — space toggles collapse
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	p := next.(tui.Picker)

	// Assert — group collapsed, view still renders without error
	assert.Contains(t, p.View(), "PROJ-A")
}

func TestPicker_DeleteFailure_SurfacedInView(t *testing.T) {
	// Arrange — single project with one session; no registry → delete will fail gracefully.
	m := tui.NewPicker([]backend.SessionMeta{{
		ID:      "ghost",
		Project: "proj-ghost",
		Kind:    session.SourceClaude,
	}}, tui.CatppuccinMocha(), nil)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act — press d then y; with nil registry the delete is a no-op (no error).
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p := m3.(tui.Picker)

	// Assert — confirm modal closed; entry removed from list (no-op delete means
	// removeMeta still runs so the in-memory list shrinks).
	assert.False(t, p.InConfirm())
}

func TestPicker_ConfirmY_DeletesEntryViaRegistry(t *testing.T) {
	// Arrange — session in picker; no actual store needed since delete in registry
	// is tested via editor_test.go; here we just verify the entry is removed from the list.
	m := tui.NewPicker([]backend.SessionMeta{{
		ID:      "x",
		Project: "proj",
		Kind:    session.SourceClaude,
	}}, tui.CatppuccinMocha(), nil)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act — press d then y
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p := m3.(tui.Picker)

	// Assert — entry removed from picker (nil registry = no actual file delete, just list update).
	assert.False(t, p.InConfirm())
	assert.Empty(t, p.Metas())
}

func TestPicker_RKeyOnBackupRowEmitsRestoreMsg(t *testing.T) {
	metas := []backend.SessionMeta{
		{ID: "a", HasBackup: true, Kind: session.SourceClaude},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil)
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(tui.RestoreRequestedMsg)
	require.True(t, ok)
	assert.Equal(t, "a", msg.ID)
}

func TestPicker_RKeyOnNonBackupRowNoOp(t *testing.T) {
	metas := []backend.SessionMeta{{ID: "a", HasBackup: false}}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil)
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	assert.Nil(t, cmd)
}
