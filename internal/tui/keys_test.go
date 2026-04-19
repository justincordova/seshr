package tui_test

import (
	"testing"

	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestReplayKeys_AllBindingsDefined(t *testing.T) {
	k := tui.DefaultReplayKeys()

	assert.NotEmpty(t, k.Next.Keys())
	assert.NotEmpty(t, k.Prev.Keys())
	assert.NotEmpty(t, k.AutoPlay.Keys())
	assert.NotEmpty(t, k.NextTopic.Keys())
	assert.NotEmpty(t, k.PrevTopic.Keys())
	assert.NotEmpty(t, k.ToggleThinking.Keys())
	assert.NotEmpty(t, k.ToggleSlim.Keys())
	assert.NotEmpty(t, k.SpeedUp.Keys())
	assert.NotEmpty(t, k.SpeedDown.Keys())
	assert.NotEmpty(t, k.Expand.Keys())
	assert.NotEmpty(t, k.Back.Keys())
	assert.NotEmpty(t, k.Quit.Keys())
}

func TestOverviewKeys_AllBindingsDefined(t *testing.T) {
	k := tui.DefaultOverviewKeys()

	assert.NotEmpty(t, k.Up.Keys())
	assert.NotEmpty(t, k.Down.Keys())
	assert.NotEmpty(t, k.Expand.Keys())
	assert.NotEmpty(t, k.FoldAll.Keys())
	assert.NotEmpty(t, k.Select.Keys())
	assert.NotEmpty(t, k.ToggleAll.Keys())
	assert.NotEmpty(t, k.Prune.Keys())
	assert.NotEmpty(t, k.Replay.Keys())
	assert.NotEmpty(t, k.Stats.Keys())
	assert.NotEmpty(t, k.Search.Keys())
	assert.NotEmpty(t, k.Back.Keys())
	assert.NotEmpty(t, k.Quit.Keys())
}
