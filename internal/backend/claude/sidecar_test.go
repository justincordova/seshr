package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeSidecar_ValidFixture(t *testing.T) {
	// Arrange
	f, err := os.Open(filepath.Join(testdataDir, "claude_live_sidecar.json"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	// Act — access via exported ReadSidecars by writing a temp dir
	dir := t.TempDir()
	require.NoError(t, copyFile(filepath.Join(testdataDir, "claude_live_sidecar.json"), filepath.Join(dir, "850.json")))

	sidecars, err := claudeBackend.ReadSidecars(dir)

	// Assert
	require.NoError(t, err)
	require.Len(t, sidecars, 1)
	assert.Equal(t, 850, sidecars[0].PID)
	assert.Equal(t, "abc123def456", sidecars[0].SessionID)
	assert.NotEmpty(t, sidecars[0].CWD)
}

func TestReadSidecars_MalformedFile_SkipsAndContinues(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, copyFile(filepath.Join(testdataDir, "claude_live_sidecar.json"), filepath.Join(dir, "good.json")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o644))

	// Act
	sidecars, err := claudeBackend.ReadSidecars(dir)

	// Assert — error on individual file is not propagated; good sidecar returned
	require.NoError(t, err)
	require.Len(t, sidecars, 1)
	assert.Equal(t, "abc123def456", sidecars[0].SessionID)
}

func TestReadSidecars_EmptyDir_ReturnsNone(t *testing.T) {
	// Arrange
	dir := t.TempDir()

	// Act
	sidecars, err := claudeBackend.ReadSidecars(dir)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, sidecars)
}
