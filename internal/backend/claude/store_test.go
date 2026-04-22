package claude_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testdataDir = "../../../testdata"

func TestStore_Load_SimpleSession(t *testing.T) {
	// Arrange — build a temp projects tree: root/myproject/mysession.jsonl
	root := t.TempDir()
	projDir := filepath.Join(root, "myproject")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), filepath.Join(projDir, "mysession.jsonl")))

	store := claudeBackend.NewStore(root)

	// Act
	sess, cur, err := store.Load(context.Background(), "mysession")

	// Assert
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, session.SourceClaude, sess.Source)
	assert.NotEmpty(t, sess.Turns)
	assert.Equal(t, session.SourceClaude, cur.Kind)
}

func TestStore_Load_UnknownSession_ReturnsError(t *testing.T) {
	// Arrange
	store := claudeBackend.NewStore(t.TempDir())

	// Act
	_, _, err := store.Load(context.Background(), "does-not-exist")

	// Assert
	assert.Error(t, err)
}

func TestStore_Scan_EmptyRoot_ReturnsNone(t *testing.T) {
	// Arrange
	store := claudeBackend.NewStore(t.TempDir())

	// Act
	metas, err := store.Scan(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Empty(t, metas)
}

func TestStore_Scan_WithSessions_ReturnsMeta(t *testing.T) {
	// Arrange
	root := t.TempDir()
	projDir := filepath.Join(root, "myproject")
	require.NoError(t, os.MkdirAll(projDir, 0o755))
	require.NoError(t, copyFile(filepath.Join(testdataDir, "simple.jsonl"), filepath.Join(projDir, "abc123.jsonl")))

	store := claudeBackend.NewStore(root)

	// Act
	metas, err := store.Scan(context.Background())

	// Assert
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "abc123", metas[0].ID)
	assert.Equal(t, session.SourceClaude, metas[0].Kind)
	assert.NotEmpty(t, metas[0].Directory)
}

// copyFile copies src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
