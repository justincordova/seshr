package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/justincordova/seshly/internal/editor"
	"github.com/justincordova/seshly/internal/parser"
	"github.com/justincordova/seshly/internal/topics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestore_ReturnsExactOriginalBytes(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "session.jsonl")
	src, err := os.ReadFile("../testdata/prune_basic.jsonl")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, src, 0o644))

	p := parser.NewClaude()
	sess, err := p.Parse(context.Background(), dst)
	require.NoError(t, err)
	ts := topics.Cluster(sess, topics.DefaultOptions())
	sel := editor.ExpandSelection(sess, ts, editor.Selection{Topics: map[int]bool{1: true}})
	require.NoError(t, editor.PruneSession(sess, sel))

	require.NoError(t, editor.Restore(dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, string(src), string(got), "restored file must match pre-prune bytes")
}
