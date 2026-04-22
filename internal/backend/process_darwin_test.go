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
	cwd, err := parseLsofCWD(raw)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "/Users/justin/cs/projects/seshr", cwd)
}

func TestParseLsofCWD_MissingNLine_ReturnsError(t *testing.T) {
	// Arrange
	input := []byte("p12345\nfcwd\n")

	// Act
	_, err := parseLsofCWD(input)

	// Assert
	assert.Error(t, err)
}

func TestParseLsofCWD_EmptyInput_ReturnsError(t *testing.T) {
	_, err := parseLsofCWD([]byte{})
	assert.Error(t, err)
}
