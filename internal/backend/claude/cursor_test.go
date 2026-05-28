package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentitiesMatch_ZeroPrev_ReturnsFalse(t *testing.T) {
	got := identitiesMatch(cursorData{}, cursorData{MtimeNs: 100, SizeBytes: 10})
	assert.False(t, got, "zero cursor must force a full reload")
}

func TestIdentitiesMatch_InodeMatch_ReturnsTrue(t *testing.T) {
	prev := cursorData{MtimeNs: 100, Inode: 42}
	cur := cursorData{MtimeNs: 200, Inode: 42}
	assert.True(t, identitiesMatch(prev, cur))
}

func TestIdentitiesMatch_InodeMismatch_ReturnsFalse(t *testing.T) {
	prev := cursorData{MtimeNs: 100, Inode: 42}
	cur := cursorData{MtimeNs: 200, Inode: 43}
	assert.False(t, identitiesMatch(prev, cur))
}

func TestIdentitiesMatch_DarwinGrowingFile_ReturnsTrue(t *testing.T) {
	// The agent appended bytes between Load and the next fast tick. Without
	// this case passing, every fast tick would full-reload and SessionView
	// would duplicate every turn.
	prev := cursorData{MtimeNs: 100, SizeBytes: 1000}
	cur := cursorData{MtimeNs: 200, SizeBytes: 1500}
	assert.True(t, identitiesMatch(prev, cur))
}

func TestIdentitiesMatch_DarwinShrunkFile_ReturnsFalse(t *testing.T) {
	// Truncation or rotation produces a smaller file; treat as rotation.
	prev := cursorData{MtimeNs: 100, SizeBytes: 1000}
	cur := cursorData{MtimeNs: 200, SizeBytes: 500}
	assert.False(t, identitiesMatch(prev, cur))
}

func TestIdentitiesMatch_DarwinMtimeRegression_ReturnsFalse(t *testing.T) {
	// Without inode information, an mtime regression looks like rotation.
	prev := cursorData{MtimeNs: 200, SizeBytes: 1000}
	cur := cursorData{MtimeNs: 100, SizeBytes: 1500}
	assert.False(t, identitiesMatch(prev, cur))
}
