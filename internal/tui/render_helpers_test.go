package tui

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHomePathDisplay_ReplacesHome(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir available")
	}

	// Act
	got := homePathDisplay(home + "/foo/bar")

	// Assert
	assert.Equal(t, "~/foo/bar", got)
}

func TestHomePathDisplay_ExactlyHome(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home dir")
	}

	// Act
	got := homePathDisplay(home)

	// Assert
	assert.Equal(t, "~", got)
}

func TestHomePathDisplay_PathWithoutHomePrefixUnchanged(t *testing.T) {
	// Arrange
	t.Setenv("HOME", "/Users/me")

	// Act
	got := homePathDisplay("/etc/hosts")

	// Assert
	assert.Equal(t, "/etc/hosts", got)
}

func TestHomePathDisplay_EmptyInputUnchanged(t *testing.T) {
	got := homePathDisplay("")

	assert.Equal(t, "", got)
}

func TestLeftTruncate_AddsEllipsis(t *testing.T) {
	got := leftTruncate("/Users/me/cs/projects/seshr", 10)

	// width=10 = 1 ellipsis + 9 trailing chars.
	assert.Equal(t, "…cts/seshr", got)
}

func TestLeftTruncate_ShortInputUnchanged(t *testing.T) {
	got := leftTruncate("hi", 10)

	assert.Equal(t, "hi", got)
}

func TestLeftTruncate_ZeroWidth(t *testing.T) {
	got := leftTruncate("anything", 0)

	assert.Equal(t, "", got)
}

func TestLeftTruncate_OneWidth(t *testing.T) {
	got := leftTruncate("anything", 1)

	assert.Equal(t, "…", got)
}
