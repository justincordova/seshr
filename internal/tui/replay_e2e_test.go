package tui_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplay_E2E_FromFixture(t *testing.T) {
	p := parser.NewClaude()
	sess, err := p.Parse(context.Background(), "../../testdata/replay_basic.jsonl")
	require.NoError(t, err)
	ts := topics.Cluster(sess, topics.DefaultOptions())
	require.NotEmpty(t, ts)

	m := tui.NewReplay(sess, ts, tui.CatppuccinMocha())
	m = m.SetSize(120, 40).(tui.Replay)

	// Advance through every turn with →
	var model tea.Model = m
	for i := 0; i < len(sess.Turns)-1; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	}

	r := model.(tui.Replay)
	assert.Equal(t, len(sess.Turns)-1, r.Cursor())
	assert.NotEmpty(t, r.View())
}
