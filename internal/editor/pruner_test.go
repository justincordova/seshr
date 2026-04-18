package editor_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justincordova/agentlens/internal/editor"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrune_OmitsSelectedTurns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","n":0}`,
		`{"type":"assistant","n":1}`,
		`{"type":"user","n":2}`,
		`{"type":"assistant","n":3}`,
	}
	require.NoError(t, os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	sess := &parser.Session{
		Path: src,
		Turns: []parser.Turn{
			{RawIndex: 0}, {RawIndex: 1}, {RawIndex: 2}, {RawIndex: 3},
		},
	}
	dst := filepath.Join(t.TempDir(), "out.jsonl")

	err := editor.Prune(sess, editor.Selection{Turns: map[int]bool{2: true, 3: true}}, dst)

	require.NoError(t, err)
	body, err := os.ReadFile(dst)
	require.NoError(t, err)
	result := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	assert.Len(t, result, 2)
	assert.Contains(t, result[0], `"n":0`)
	assert.Contains(t, result[1], `"n":1`)
}

func TestPrune_EmptySelectionKeepsAllTurns(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(src, []byte(`{"type":"user"}`+"\n"+`{"type":"assistant"}`+"\n"), 0o644))

	sess := &parser.Session{
		Path:  src,
		Turns: []parser.Turn{{RawIndex: 0}, {RawIndex: 1}},
	}
	dst := filepath.Join(t.TempDir(), "out.jsonl")

	err := editor.Prune(sess, editor.Selection{Turns: map[int]bool{}}, dst)

	require.NoError(t, err)
	body, _ := os.ReadFile(dst)
	assert.Equal(t, 2, strings.Count(string(body), "\n"))
}

func TestPruneSession_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	original := strings.Join([]string{
		`{"type":"user","sessionId":"s","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"first"}}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","content":"ok"}}`,
		`{"type":"user","sessionId":"s","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":"second"}}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2025-01-01T00:00:03Z","message":{"role":"assistant","content":"done"}}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	p := parser.NewClaude()
	sess, err := p.Parse(context.Background(), path)
	require.NoError(t, err)

	err = editor.PruneSession(sess, editor.Selection{Turns: map[int]bool{2: true, 3: true}})

	require.NoError(t, err)
	after, err := p.Parse(context.Background(), path)
	require.NoError(t, err)
	assert.Len(t, after.Turns, 2)
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, original, string(bak))
}

func TestPruneSession_LockedReturnsErrLocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o644))
	l, err := editor.TryLock(path)
	require.NoError(t, err)
	defer l.Release()
	sess := &parser.Session{Path: path}

	err = editor.PruneSession(sess, editor.Selection{})

	assert.ErrorIs(t, err, editor.ErrLocked)
}

func TestPrune_UsesRawIndexNotTurnIndex(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","n":0}`,
		`MALFORMED`,
		`{"type":"assistant","n":1}`,
		`{"type":"user","n":2}`,
	}
	require.NoError(t, os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	sess := &parser.Session{
		Path: src,
		Turns: []parser.Turn{
			{RawIndex: 0},
			{RawIndex: 2},
			{RawIndex: 3},
		},
	}
	dst := filepath.Join(t.TempDir(), "out.jsonl")

	err := editor.Prune(sess, editor.Selection{Turns: map[int]bool{1: true}}, dst)

	require.NoError(t, err)
	body, err := os.ReadFile(dst)
	require.NoError(t, err)
	result := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	assert.Len(t, result, 3)
	assert.Contains(t, result[0], `"n":0`)
	assert.Contains(t, result[1], "MALFORMED")
	assert.Contains(t, result[2], `"n":2`)
}

func TestPrune_RemovesAttachedToolResultLines(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","n":0}`,
		`{"type":"assistant","n":1}`,
		`{"type":"tool_result","n":"tr"}`,
		`{"type":"user","n":2}`,
	}
	require.NoError(t, os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	sess := &parser.Session{
		Path: src,
		Turns: []parser.Turn{
			{RawIndex: 0},
			{RawIndex: 1, ExtraLineIndices: []int{2}},
			{RawIndex: 3},
		},
	}
	dst := filepath.Join(t.TempDir(), "out.jsonl")

	err := editor.Prune(sess, editor.Selection{Turns: map[int]bool{1: true}}, dst)

	require.NoError(t, err)
	body, err := os.ReadFile(dst)
	require.NoError(t, err)
	result := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	assert.Len(t, result, 2)
	assert.Contains(t, result[0], `"n":0`)
	assert.Contains(t, result[1], `"n":2`)
}

func TestPruneSession_EndToEndWithToolResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	original := strings.Join([]string{
		`{"type":"user","sessionId":"s","timestamp":"2025-01-01T00:00:00Z","message":{"role":"user","content":"first"}}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2025-01-01T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}],"usage":{"input_tokens":10,"output_tokens":5}}}`,
		`{"type":"tool_result","tool_use_id":"t1","sessionId":"s","timestamp":"2025-01-01T00:00:02Z","message":{"role":"user","content":"file.txt"}}`,
		`{"type":"user","sessionId":"s","timestamp":"2025-01-01T00:00:03Z","message":{"role":"user","content":"second"}}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2025-01-01T00:00:04Z","message":{"role":"assistant","content":"done"},"usage":{"input_tokens":5,"output_tokens":3}}`,
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	p := parser.NewClaude()
	sess, err := p.Parse(context.Background(), path)
	require.NoError(t, err)

	err = editor.PruneSession(sess, editor.Selection{Turns: map[int]bool{0: true, 1: true}})

	require.NoError(t, err)
	after, err := p.Parse(context.Background(), path)
	require.NoError(t, err)
	assert.Len(t, after.Turns, 2)
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Equal(t, original, string(bak))
}
