package tui

import (
	"testing"

	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestShortDisplayID_ClaudeUUID(t *testing.T) {
	got := shortDisplayID(session.SourceClaude, "bb859dee-0744-4c12-9a3e-aaaa")

	assert.Equal(t, "sesh_bb859d", got)
}

func TestShortDisplayID_OpenCodeID(t *testing.T) {
	got := shortDisplayID(session.SourceOpenCode, "23afb0xy")

	assert.Equal(t, "sesh_23afb0", got)
}

func TestShortDisplayID_ShortInput(t *testing.T) {
	got := shortDisplayID(session.SourceClaude, "abc")

	assert.Equal(t, "sesh_abc", got)
}

func TestShortDisplayID_Empty(t *testing.T) {
	got := shortDisplayID(session.SourceClaude, "")

	assert.Equal(t, "sesh_", got)
}

func TestShortDisplayID_StripsDashesAndLowercases(t *testing.T) {
	got := shortDisplayID(session.SourceClaude, "AB-CD-EF-GH")

	assert.Equal(t, "sesh_abcdef", got)
}
