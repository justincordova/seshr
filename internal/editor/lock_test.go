package editor_test

import (
	"path/filepath"
	"testing"

	"github.com/justincordova/seshly/internal/editor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryLock_Succeeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lock, err := editor.TryLock(path)

	require.NoError(t, err)
	require.NotNil(t, lock)
	assert.NoError(t, lock.Release())
}

func TestTryLock_SecondAcquireFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	first, err := editor.TryLock(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Release() })

	second, err := editor.TryLock(path)

	assert.Nil(t, second)
	assert.ErrorIs(t, err, editor.ErrLocked)
}
