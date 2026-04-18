package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/seshr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDir_ReturnsSeshrSubdir(t *testing.T) {
	// Arrange / Act
	dir, err := config.Dir()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ".seshr", filepath.Base(dir))
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

func TestLoad_MissingFile_ReturnsDefaults(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Act
	cfg, err := config.Load()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "catppuccin", cfg.Theme)
	assert.Equal(t, 3*60, cfg.GapThresholdSeconds)
	assert.Equal(t, 200_000, cfg.DefaultContextWindow)
}

func TestLoadSave_RoundTrip_PreservesValues(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfg := config.Default()
	cfg.Theme = "nord"
	cfg.GapThresholdSeconds = 120

	// Act
	require.NoError(t, config.Save(cfg))
	got, err := config.Load()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "nord", got.Theme)
	assert.Equal(t, 120, got.GapThresholdSeconds)
}

func TestLoad_UnknownField_IgnoredWithoutError(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, ".seshr")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config.json"),
		[]byte(`{"theme":"dracula","future_option":42}`),
		0o644,
	))

	// Act
	cfg, err := config.Load()

	// Assert — unknown fields are ignored per SPEC §4.3 schema evolution rule
	require.NoError(t, err)
	assert.Equal(t, "dracula", cfg.Theme)
}
