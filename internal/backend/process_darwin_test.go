//go:build darwin

package backend

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLsofCWD_ExtractsCWD(t *testing.T) {
	// Arrange
	raw, err := os.ReadFile("../../testdata/lsof_cwd_output.txt")
	require.NoError(t, err)

	// Act
	cwd, err := parseLsofCWD(raw, 12345)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "/Users/justin/cs/projects/seshr", cwd)
}

func TestParseLsofCWD_MissingNLine_ReturnsError(t *testing.T) {
	// Arrange
	input := []byte("p12345\nfcwd\n")

	// Act
	_, err := parseLsofCWD(input, 12345)

	// Assert
	assert.Error(t, err)
}

func TestParseLsofCWD_EmptyInput_ReturnsError(t *testing.T) {
	_, err := parseLsofCWD([]byte{}, 1)
	assert.Error(t, err)
}

// Without -a, lsof returns blocks for every process. Defends against the
// regression where parseLsofCWD returned the first n-line regardless of pid.
func TestParseLsofCWD_MultipleBlocks_ScopesToWantPID(t *testing.T) {
	input := []byte("p400\nfcwd\nn/\np23758\nfcwd\nn/Users/justin/cs/projects/seshr\np580\nfcwd\nn/var/empty\n")

	cwd, err := parseLsofCWD(input, 23758)

	require.NoError(t, err)
	assert.Equal(t, "/Users/justin/cs/projects/seshr", cwd)
}

func TestParseLsofCWD_PIDNotPresent_ReturnsError(t *testing.T) {
	input := []byte("p400\nfcwd\nn/\np580\nfcwd\nn/var\n")

	_, err := parseLsofCWD(input, 23758)

	assert.Error(t, err)
}
