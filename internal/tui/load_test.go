package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/seshr/internal/backend"
	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSessionByIDCmd_ValidSession_ReturnsLoadedMsg(t *testing.T) {
	// Arrange — write a session file in a tmp projects dir.
	root := t.TempDir()
	proj := filepath.Join(root, "myproject")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	path := filepath.Join(proj, "sess.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(
		`{"type":"user","message":{"role":"user","content":"hi"},"uuid":"u1","timestamp":"2026-03-20T10:00:00Z","sessionId":"sess"}`+"\n"+
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":5,"output_tokens":2}},"uuid":"a1","timestamp":"2026-03-20T10:00:01Z","sessionId":"sess"}`+"\n",
	), 0o644))

	reg := backend.NewRegistry()
	store := claudeBackend.NewStore(root)
	reg.RegisterStore(store)

	meta := backend.SessionMeta{ID: "sess", Kind: session.SourceClaude}

	// Ensure store can load it.
	s, _, err := store.Load(context.Background(), "sess")
	require.NoError(t, err)
	require.NotNil(t, s)

	// Act
	cmd := tui.LoadSessionByIDCmd(meta, reg, 0)
	msg := cmd()

	// Assert
	loaded, ok := msg.(tui.SessionLoadedMsg)
	require.True(t, ok, "expected SessionLoadedMsg, got %T", msg)
	assert.NotNil(t, loaded.Session)
	assert.Len(t, loaded.Session.Turns, 2)
	assert.NotEmpty(t, loaded.Topics)
}

func TestLoadSessionByIDCmd_NilRegistry_ReturnsErrMsg(t *testing.T) {
	// Act
	meta := backend.SessionMeta{ID: "x", Kind: session.SourceClaude}
	cmd := tui.LoadSessionByIDCmd(meta, nil, 0)
	msg := cmd()

	// Assert
	errMsg, ok := msg.(tui.SessionLoadErrMsg)
	require.True(t, ok, "expected SessionLoadErrMsg, got %T", msg)
	assert.Error(t, errMsg.Err)
}

func TestLoadSessionByIDCmd_UnknownKind_ReturnsErrMsg(t *testing.T) {
	// Act — registry with no store for OpenCode
	reg := backend.NewRegistry()
	meta := backend.SessionMeta{ID: "x", Kind: session.SourceOpenCode}
	cmd := tui.LoadSessionByIDCmd(meta, reg, 0)
	msg := cmd()

	// Assert
	errMsg, ok := msg.(tui.SessionLoadErrMsg)
	require.True(t, ok, "expected SessionLoadErrMsg, got %T", msg)
	assert.Error(t, errMsg.Err)
}
