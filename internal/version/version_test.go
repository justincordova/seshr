package version_test

import (
	"testing"

	"github.com/justincordova/seshr/internal/version"
	"github.com/stretchr/testify/assert"
)

func TestVersion_Default_IsNonEmpty(t *testing.T) {
	// Arrange / Act / Assert
	assert.NotEmpty(t, version.Version)
}
