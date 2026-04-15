package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/agentlens/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_EmptyRoot_ReturnsEmpty(t *testing.T) {
	// Arrange
	root := t.TempDir()

	// Act
	got, err := parser.Scan(root)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestScan_MissingRoot_ReturnsEmptyNotError(t *testing.T) {
	// Arrange — nonexistent dir is a normal first-run case
	root := filepath.Join(t.TempDir(), "does-not-exist")

	// Act
	got, err := parser.Scan(root)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestScan_ProjectsWithSessions_ReturnsMeta(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projA := filepath.Join(root, "-Users-someone-project-a")
	projB := filepath.Join(root, "-Users-someone-project-b")
	require.NoError(t, os.MkdirAll(projA, 0o755))
	require.NoError(t, os.MkdirAll(projB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projA, "abc.jsonl"), []byte(`{"type":"user"}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projA, "def.jsonl"), []byte(`{"type":"user"}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projB, "ghi.jsonl"), []byte(`{"type":"user"}`+"\n"), 0o644))
	// Non-jsonl sibling — must be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(projA, "notes.txt"), []byte("ignore me"), 0o644))

	// Act
	got, err := parser.Scan(root)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 3)
	for _, m := range got {
		assert.NotEmpty(t, m.ID)
		assert.NotEmpty(t, m.Path)
		assert.NotEmpty(t, m.Project)
		assert.False(t, m.ModifiedAt.IsZero())
	}
}

func TestScan_DetectsBackupSibling(t *testing.T) {
	// Arrange — SPEC §4.5 restore relies on this flag being set at scan time.
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "x.jsonl"), []byte(`{}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "x.jsonl.bak"), []byte(`{}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "y.jsonl"), []byte(`{}`+"\n"), 0o644))

	// Act
	got, err := parser.Scan(root)

	// Assert
	require.NoError(t, err)
	require.Len(t, got, 2)
	backed, plain := false, false
	for _, m := range got {
		if m.HasBackup {
			backed = true
			assert.Equal(t, "x", m.ID)
		} else {
			plain = true
			assert.Equal(t, "y", m.ID)
		}
	}
	assert.True(t, backed && plain, "scan should flag .bak sibling correctly")
}
