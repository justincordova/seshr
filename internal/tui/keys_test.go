package tui_test

import (
	"testing"

	"github.com/justincordova/agentlens/internal/tui"
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
	assert.NotEmpty(t, k.ToggleWrap.Keys())
	assert.NotEmpty(t, k.Expand.Keys())
	assert.NotEmpty(t, k.Back.Keys())
	assert.NotEmpty(t, k.Quit.Keys())
}
