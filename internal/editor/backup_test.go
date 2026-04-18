package editor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/agentlens/internal/editor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBackup_CopiesExactBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	original := []byte(`{"type":"user"}` + "\n")
	require.NoError(t, os.WriteFile(src, original, 0o644))

	err := editor.CreateBackup(src)

	require.NoError(t, err)
	got, err := os.ReadFile(src + ".bak")
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestAtomicReplace_ReplacesDst(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new.jsonl")
	dst := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(src, []byte("NEW"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("OLD"), 0o644))

	err := editor.AtomicReplace(src, dst)

	require.NoError(t, err)
	got, _ := os.ReadFile(dst)
	assert.Equal(t, "NEW", string(got))
	_, statErr := os.Stat(src)
	assert.True(t, os.IsNotExist(statErr), "src should be gone after rename")
}

func TestRestore_CopiesBakOverOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("PRUNED"), 0o644))
	require.NoError(t, os.WriteFile(path+".bak", []byte("ORIGINAL"), 0o644))

	err := editor.Restore(path)

	require.NoError(t, err)
	got, _ := os.ReadFile(path)
	assert.Equal(t, "ORIGINAL", string(got))
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(bak))
}

func TestRestore_NoBackupReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("X"), 0o644))

	err := editor.Restore(path)

	assert.ErrorIs(t, err, editor.ErrNoBackup)
}
