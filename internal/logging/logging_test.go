package logging_test

import (
	"path/filepath"
	"testing"

	"github.com/justincordova/seshly/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_HomeOverridden_CreatesLogFile(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Act
	err := logging.Init(true)

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(tmp, ".seshly", "debug.log"))
}
