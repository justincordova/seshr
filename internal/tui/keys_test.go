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

func TestEditorKeys_AllBindingsDefined(t *testing.T) {
	k := tui.DefaultEditorKeys()

	assert.NotEmpty(t, k.Toggle.Keys())
	assert.NotEmpty(t, k.SelectAll.Keys())
	assert.NotEmpty(t, k.SelectNone.Keys())
	assert.NotEmpty(t, k.Prune.Keys())
	assert.NotEmpty(t, k.Expand.Keys())
	assert.NotEmpty(t, k.Cancel.Keys())
	assert.NotEmpty(t, k.Quit.Keys())
}
