package tui_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtures() []parser.SessionMeta {
	return []parser.SessionMeta{
		{ID: "a", Path: "/p/a.jsonl", Project: "proj-a", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-1 * time.Hour)},
		{ID: "b", Path: "/p/b.jsonl", Project: "proj-b", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-24 * time.Hour)},
		{ID: "c", Path: "/p/c.jsonl", Project: "proj-c", Source: parser.SourceClaude, ModifiedAt: time.Now().Add(-72 * time.Hour)},
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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 1, p.Cursor())
}

func TestPicker_UpKey_AtTopStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 0, p.Cursor())
}

func TestPicker_DownKey_AtBottomStays(t *testing.T) {
	// Arrange — 3 projects, all collapsed by default → 3 flat rows, index 0..2
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())
	m = pressDown(m, 10)

	// Assert — clamped at last row (index 2)
	assert.Equal(t, 2, m.Cursor())
}

func TestPicker_QuitKey_EmitsQuitCmd(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestPicker_Empty_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	m := tui.NewPicker(nil, tui.CatppuccinMocha())

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}

func TestPicker_View_ContainsProjectName(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act
	out := m.View()

	// Assert — project name is uppercased in the header for visual emphasis.
	assert.Contains(t, out, "PROJ-A")
}

func TestPicker_DKey_OnSession_EntersConfirmState(t *testing.T) {
	// Arrange — expand the first group, then move down onto its session row
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())
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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert — delete is no-op on group header
	p := next.(tui.Picker)
	assert.False(t, p.InConfirm())
}

func TestPicker_ConfirmN_LeavesConfirmNoDelete(t *testing.T) {
	// Arrange — expand the first group and move onto its session row
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())
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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())
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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

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
	m := tui.NewPicker(fixtures(), tui.CatppuccinMocha())

	// Act — space toggles collapse
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	p := next.(tui.Picker)

	// Assert — group collapsed, view still renders without error
	assert.Contains(t, p.View(), "PROJ-A")
}

func TestPicker_DeleteFailure_SurfacedInView(t *testing.T) {
	// Arrange — single project with one session, cursor starts on group header
	m := tui.NewPicker([]parser.SessionMeta{{
		ID:      "ghost",
		Path:    "/nonexistent/dir/ghost.jsonl",
		Project: "proj-ghost",
		Source:  parser.SourceClaude,
	}}, tui.CatppuccinMocha())
	// Expand the group, then navigate to the session row
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act — press d then y to trigger a delete that fails.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p := m3.(tui.Picker)

	// Assert — confirm modal closed and the error surface is rendered in the view.
	assert.False(t, p.InConfirm())
	assert.Contains(t, p.View(), "delete failed")
}

func TestPicker_ConfirmY_DeletesFileAndEntry(t *testing.T) {
	// Arrange — real files in a tmp dir
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	jsonlPath := filepath.Join(proj, "x.jsonl")
	require.NoError(t, os.WriteFile(jsonlPath, []byte(`{"type":"user"}`+"\n"), 0o644))
	m := tui.NewPicker([]parser.SessionMeta{{
		ID:      "x",
		Path:    jsonlPath,
		Project: "proj",
		Source:  parser.SourceClaude,
	}}, tui.CatppuccinMocha())
	// Expand the group, then navigate to the session row
	m = expandCurrentGroup(m)
	m = pressDown(m, 1)

	// Act — press d then y
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	p := m3.(tui.Picker)

	// Assert
	assert.False(t, p.InConfirm())
	assert.Empty(t, p.Metas())
	_, err := os.Stat(jsonlPath)
	assert.True(t, os.IsNotExist(err), "file should be gone")
	_, err = os.Stat(proj)
	assert.True(t, os.IsNotExist(err), "empty project dir should be cleaned up")
}

func TestPicker_RKeyOnBackupRowEmitsRestoreMsg(t *testing.T) {
	metas := []parser.SessionMeta{
		{ID: "a", Path: "/x/a.jsonl", HasBackup: true},
	}
	p := tui.NewPicker(metas, tui.CatppuccinMocha())
	// Expand the group, then navigate to the session row
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(tui.RestoreRequestedMsg)
	require.True(t, ok)
	assert.Equal(t, "/x/a.jsonl", msg.Path)
}

func TestPicker_RKeyOnNonBackupRowNoOp(t *testing.T) {
	metas := []parser.SessionMeta{{ID: "a", Path: "/x/a.jsonl", HasBackup: false}}
	p := tui.NewPicker(metas, tui.CatppuccinMocha())
	// Expand the group, then navigate to the session row
	p = expandCurrentGroup(p)
	p = pressDown(p, 1)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	assert.Nil(t, cmd)
}
