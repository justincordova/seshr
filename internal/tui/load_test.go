package tui_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSession_ValidFile_ReturnsLoadedMsg(t *testing.T) {
	// Arrange
	tmp := t.TempDir()
	path := filepath.Join(tmp, "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"hi"},"uuid":"u1","timestamp":"2026-03-20T10:00:00Z","sessionId":"sess"}`+"\n"+
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":2}},"uuid":"a1","timestamp":"2026-03-20T10:00:01Z","sessionId":"sess"}`+"\n",
	), 0o644))

	// Act
	cmd := tui.LoadSessionCmd(path)
	msg := cmd()

	// Assert
	loaded, ok := msg.(tui.SessionLoadedMsg)
	require.True(t, ok, "expected SessionLoadedMsg, got %T", msg)
	assert.NotNil(t, loaded.Session)
	assert.Len(t, loaded.Session.Turns, 2)
	assert.NotEmpty(t, loaded.Topics)
}

func TestLoadSession_InvalidFile_ReturnsErrMsg(t *testing.T) {
	// Act
	cmd := tui.LoadSessionCmd("/nonexistent/path.jsonl")
	msg := cmd()

	// Assert
	errMsg, ok := msg.(tui.SessionLoadErrMsg)
	require.True(t, ok, "expected SessionLoadErrMsg, got %T", msg)
	assert.Error(t, errMsg.Err)
	assert.Equal(t, "/nonexistent/path.jsonl", errMsg.Path)
}
