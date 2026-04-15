package config_test

import (
	"path/filepath"
	"testing"

	"github.com/justincordova/agentlens/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDir_ReturnsAgentlensSubdir(t *testing.T) {
	// Arrange / Act
	dir, err := config.Dir()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ".agentlens", filepath.Base(dir))
}

func TestEnsureDir_HomeOverridden_CreatesDir(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Act
	dir, err := config.EnsureDir()

	// Assert
	require.NoError(t, err)
	assert.DirExists(t, dir)
}
