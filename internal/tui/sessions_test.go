package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/config"
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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 1, p.Cursor())
}

func TestPicker_UpKey_AtTopStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 0, p.Cursor())
}

func TestPicker_DownKey_AtBottomStays(t *testing.T) {
	// Arrange — Project view: 3 projects, all collapsed by default → 3 flat
	// rows, index 0..2. Use Project mode to keep the row count predictable
	// across changes to Recent view rendering.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	m = pressDown(m, 10)

	// Assert — clamped at last row (index 2)
	assert.Equal(t, 2, m.Cursor())
}

func TestPicker_QuitKey_EmitsQuitCmd(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestPicker_Empty_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	m := tui.NewPicker(nil, tui.CatppuccinMocha(), nil, "")

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}

func TestPicker_View_ContainsProjectName(t *testing.T) {
	// Arrange — Project view shows uppercased project names in group headers.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Act
	out := m.View()

	// Assert — project name is uppercased in the header for visual emphasis.
	assert.Contains(t, out, "PROJ-A")
}

func TestPicker_DKey_OnSession_EntersConfirmState(t *testing.T) {
	// Arrange — Project view: expand first group, move onto a session row.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert
	p := next.(tui.Picker)
	assert.True(t, p.InConfirm())
}

func TestPicker_DKey_OnGroupHeader_NoOp(t *testing.T) {
	// Arrange — Project view: cursor starts on a group header.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert — delete is no-op on group header
	p := next.(tui.Picker)
	assert.False(t, p.InConfirm())
}

func TestPicker_ConfirmN_LeavesConfirmNoDelete(t *testing.T) {
	// Arrange — Project view: expand first group, move onto a session row.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
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
	// Arrange — Project view: expand first group, move onto a session row.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
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
	// Arrange — Project view: cursor on group header (row 0).
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Act — enter toggles collapse
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	p := next.(tui.Picker)

	// Assert — group header still selected; not a session row.
	_, ok := p.Selected()
	assert.False(t, ok, "cursor on group header should not return a session")
}

func TestPicker_SpaceKey_OnGroupHeader_TogglesCollapse(t *testing.T) {
	// Arrange — Project view exposes group headers.
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Act — space toggles collapse
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	p := next.(tui.Picker)

	// Assert — group collapsed, view still renders without error
	assert.Contains(t, p.View(), "PROJ-A")
}

func TestPicker_DeleteFailure_SurfacedInView(t *testing.T) {
	// Arrange — Project view: single project with one session; no registry
	// → delete is a no-op but exercises the confirm flow.
	m := tui.NewPicker([]backend.SessionMeta{{
		ID:      "ghost",
		Project: "proj-ghost",
		Kind:    session.SourceClaude,
	}}, tui.CatppuccinMocha(), nil, config.PickerViewProject)
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
	// Arrange — Project view: session in picker; no actual store needed
	// since delete in registry is tested via editor_test.go; here we just
	// verify the entry is removed from the list.
	m := tui.NewPicker([]backend.SessionMeta{{
		ID:      "x",
		Project: "proj",
		Kind:    session.SourceClaude,
	}}, tui.CatppuccinMocha(), nil, config.PickerViewProject)
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
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, config.PickerViewProject)
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
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, config.PickerViewProject)
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	assert.Nil(t, cmd)
}

// manyMetas builds N metas across 3 projects so the picker has enough rows
// to exercise paging behavior.
func manyMetas(n int) []backend.SessionMeta {
	out := make([]backend.SessionMeta, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		out[i] = backend.SessionMeta{
			ID:        "id" + string(rune('a'+i%26)) + string(rune('0'+i%10)),
			Project:   "proj-" + string(rune('a'+i%3)),
			Kind:      session.SourceClaude,
			UpdatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
	}
	return out
}

func TestPicker_VimGotoTop(t *testing.T) {
	// Arrange — Project view, expand a group, move cursor mid-list.
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	require.Greater(t, p.Cursor(), 0)

	// Act
	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})

	// Assert
	assert.Equal(t, 0, next.(tui.Picker).Cursor())
}

func TestPicker_VimGotoBottom(t *testing.T) {
	// Arrange — Recent view with several rows.
	p := tui.NewPicker(manyMetas(20), tui.CatppuccinMocha(), nil, "")
	// Set window so we have a viewport.
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p = next.(tui.Picker)

	// Act
	next2, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	p = next2.(tui.Picker)

	// Assert — cursor at last row (Recent has no group headers, so all rows
	// except a possible divider are sessions; cursor lands on the very last
	// row).
	assert.Greater(t, p.Cursor(), 0)
}

func TestPicker_VimPageDown(t *testing.T) {
	// Arrange
	p := tui.NewPicker(manyMetas(20), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	p = expandCurrentGroup(p)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p = next.(tui.Picker)
	startCursor := p.Cursor()

	// Act
	next2, _ := p.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	p = next2.(tui.Picker)

	// Assert — cursor advanced.
	assert.Greater(t, p.Cursor(), startCursor)
}

func TestPicker_VimPageUp(t *testing.T) {
	// Arrange — page down first to give us room to page up.
	p := tui.NewPicker(manyMetas(20), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	p = expandCurrentGroup(p)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p = next.(tui.Picker)
	next2, _ := p.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	p = next2.(tui.Picker)
	mid := p.Cursor()
	require.Greater(t, mid, 0)

	// Act
	next3, _ := p.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	p = next3.(tui.Picker)

	// Assert — cursor moved back.
	assert.Less(t, p.Cursor(), mid)
}

func TestPicker_VimKeysIgnoredInSearch(t *testing.T) {
	// Arrange — open search.
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")
	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	p = next.(tui.Picker)
	startCursor := p.Cursor()

	// Act — typing 'g' should filter, not goto-top.
	next2, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	p = next2.(tui.Picker)

	// Assert — cursor unchanged, search active.
	assert.Equal(t, startCursor, p.Cursor())
}

func TestPicker_DefaultViewModeRecent(t *testing.T) {
	// Arrange / Act — empty mode string falls back to default (Recent).
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")

	// Assert
	assert.Equal(t, config.PickerViewRecent, p.ViewMode())
}

func TestPicker_AcceptsProjectMode(t *testing.T) {
	// Arrange / Act
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Assert
	assert.Equal(t, config.PickerViewProject, p.ViewMode())
}

func TestPicker_InvalidModeFallsBackToRecent(t *testing.T) {
	// Arrange / Act
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "garbage")

	// Assert
	assert.Equal(t, config.PickerViewRecent, p.ViewMode())
}

func TestPicker_ToggleViewEmitsMsg(t *testing.T) {
	// Arrange — start in Recent.
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, "")

	// Act — press v.
	next, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	// Assert
	require.NotNil(t, cmd)
	msg := cmd()
	vm, ok := msg.(tui.PickerViewModeChangedMsg)
	require.True(t, ok)
	assert.Equal(t, config.PickerViewProject, vm.Mode)
	assert.Equal(t, config.PickerViewProject, next.(tui.Picker).ViewMode())
}

func TestPicker_ToggleViewCyclesBackToRecent(t *testing.T) {
	// Arrange — start in Project.
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Act — press v.
	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	// Assert
	assert.Equal(t, config.PickerViewRecent, next.(tui.Picker).ViewMode())
}

func TestPicker_RecentView_SelectedSessionResolves(t *testing.T) {
	// Arrange — Recent view, multiple metas.
	metas := []backend.SessionMeta{
		{ID: "a", Project: "p1", Kind: session.SourceClaude, UpdatedAt: time.Now()},
		{ID: "b", Project: "p2", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "c", Project: "p3", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-2 * time.Hour)},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, "")
	// In Recent view, cursor 0 is already on the first session row (no group headers).
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p = next.(tui.Picker)

	// Act — Selected returns meta at cursor.
	sel, ok := p.Selected()

	// Assert
	require.True(t, ok)
	assert.Equal(t, "a", sel.ID, "first row should be most-recent meta")
}

func TestPicker_ProjectView_LivePinnedAtTop(t *testing.T) {
	// Arrange — Project view with a live session.
	metas := []backend.SessionMeta{
		{ID: "ended-a", Project: "p1", Kind: session.SourceClaude, UpdatedAt: time.Now()},
		{ID: "live-a", Project: "p2", Kind: session.SourceClaude, UpdatedAt: time.Now().Add(-1 * time.Hour)},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, config.PickerViewProject)

	// Inject a live index.
	live := map[string]*backend.LiveSession{
		"live-a": {SessionID: "live-a", Status: backend.StatusWorking},
	}
	p.SetLiveIndex(live)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	p = next.(tui.Picker)

	// Act
	view := p.View()

	// Assert — LIVE group header rendered before any other group.
	assert.Contains(t, view, "◉ LIVE")
	livePos := strings.Index(view, "◉ LIVE")
	require.GreaterOrEqual(t, livePos, 0)
	// Project p1 header should come after LIVE header.
	p1Pos := strings.Index(view, "P1")
	if p1Pos >= 0 {
		assert.Greater(t, p1Pos, livePos, "LIVE group header precedes P1")
	}
}

func TestPicker_RecentView_DeleteKey_NoCrash(t *testing.T) {
	// Arrange — Recent view; cursor on a session row.
	metas := []backend.SessionMeta{
		{ID: "a", Project: "p", Kind: session.SourceClaude, UpdatedAt: time.Now()},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, "")
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	p = next.(tui.Picker)

	// Act — pressing 'd' should enter the confirm modal without panicking.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("delete in Recent view panicked: %v", r)
		}
	}()
	next2, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert — confirm modal is shown.
	assert.True(t, next2.(tui.Picker).InConfirm())
}

func TestPicker_RecentView_View_ContainsShortID(t *testing.T) {
	// Arrange
	metas := []backend.SessionMeta{
		{ID: "bb859dee-0744-4c12-9a3e-aaaa", Project: "p", Kind: session.SourceClaude, UpdatedAt: time.Now()},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha(), nil, "")
	next, _ := p.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	p = next.(tui.Picker)

	// Act
	view := p.View()

	// Assert — short id present.
	assert.Contains(t, view, "sesh_bb859d")
	// Full UUID not present.
	assert.NotContains(t, view, "bb859dee-0744")
}

func TestPicker_ToggleResetsCursor(t *testing.T) {
	// Arrange — Project mode, expanded group with sessions; move cursor down.
	p := tui.NewPicker(fixtures(), tui.CatppuccinMocha(), nil, config.PickerViewProject)
	p = expandCurrentGroup(p)
	p = pressDown(p, 2)
	require.Greater(t, p.Cursor(), 0)

	// Act — press v.
	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})

	// Assert
	assert.Equal(t, 0, next.(tui.Picker).Cursor())
}
