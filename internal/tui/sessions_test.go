package tui_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/tui"
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

func TestPicker_DownKey_MovesCursor(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 1, p.Cursor())
}

func TestPicker_UpKey_AtTopStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	p := next.(tui.Picker)
	assert.Equal(t, 0, p.Cursor())
}

func TestPicker_DownKey_AtBottomStays(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())
	for i := 0; i < 5; i++ {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = m2.(tui.Picker)
	}

	// Assert — clamped at len-1
	assert.Equal(t, 2, m.Cursor())
}

func TestPicker_QuitKey_EmitsQuitCmd(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestPicker_Empty_ShowsEmptyMessage(t *testing.T) {
	// Arrange
	m := tui.NewPicker(nil)

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "No sessions found")
}

func TestPicker_View_ContainsProjectName(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	out := m.View()

	// Assert
	assert.Contains(t, out, "proj-a")
}

func TestPicker_DKey_EntersConfirmState(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Assert
	p := next.(tui.Picker)
	assert.True(t, p.InConfirm())
}

func TestPicker_ConfirmN_LeavesConfirmNoDelete(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(tui.Picker)

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	// Assert
	p := next.(tui.Picker)
	assert.False(t, p.InConfirm())
	assert.Len(t, p.Metas(), 3, "no entries should be removed on cancel")
}

func TestPicker_EnterKey_EmitsOpenSessionMsg(t *testing.T) {
	// Arrange
	m := tui.NewPicker(fixtures())

	// Act
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	require.NotNil(t, cmd)
	msg := cmd()
	open, ok := msg.(tui.OpenSessionMsg)
	require.True(t, ok, "expected OpenSessionMsg, got %T", msg)
	assert.Equal(t, "a", open.Meta.ID)
}

func TestPicker_DeleteFailure_SurfacedInView(t *testing.T) {
	// Arrange — meta points at a path that does not exist, so os.Remove fails.
	m := tui.NewPicker([]parser.SessionMeta{{
		ID:      "ghost",
		Path:    "/nonexistent/dir/ghost.jsonl",
		Project: "proj-ghost",
		Source:  parser.SourceClaude,
	}})

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
	}})

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
	p := tui.NewPicker(metas)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(tui.RestoreRequestedMsg)
	require.True(t, ok)
	assert.Equal(t, "/x/a.jsonl", msg.Path)
}

func TestPicker_RKeyOnNonBackupRowNoOp(t *testing.T) {
	metas := []parser.SessionMeta{{ID: "a", Path: "/x/a.jsonl", HasBackup: false}}
	p := tui.NewPicker(metas)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	assert.Nil(t, cmd)
}
