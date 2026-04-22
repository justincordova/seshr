package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	"github.com/justincordova/seshr/internal/editor"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneRoundTrip_DropsSecondTopic(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "session.jsonl")
	src, err := os.ReadFile("../testdata/prune_basic.jsonl")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, src, 0o644))

	p := claudeBackend.NewClaude()
	sess, err := p.Parse(context.Background(), dst)
	require.NoError(t, err)
	ts := topics.Cluster(sess, topics.DefaultOptions())
	require.Len(t, ts, 2, "fixture should parse into 2 topics")

	sel := editor.ExpandSelection(sess, ts, editor.Selection{Topics: map[int]bool{1: true}})
	require.NoError(t, editor.PruneSession(sess, sel))

	after, err := p.Parse(context.Background(), dst)
	require.NoError(t, err)
	tsAfter := topics.Cluster(after, topics.DefaultOptions())
	assert.Len(t, tsAfter, 1)
	_, err = os.Stat(dst + ".bak")
	assert.NoError(t, err)
}
