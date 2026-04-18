package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSession() *parser.Session {
	return &parser.Session{
		ID: "s1",
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Content: "hello", Timestamp: time.Unix(100, 0)},
			{Role: parser.RoleAssistant, Content: "hi", Timestamp: time.Unix(110, 0)},
			{Role: parser.RoleUser, Content: "next", Timestamp: time.Unix(120, 0)},
		},
	}
}

func sampleTopics() []topics.Topic {
	return []topics.Topic{
		{Label: "Greet", TurnIndices: []int{0, 1}},
		{Label: "Next", TurnIndices: []int{2}},
	}
}

func TestReplay_NewDefaults(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	assert.Equal(t, 0, m.Cursor())
	assert.False(t, m.ThinkingVisible())
	assert.False(t, m.AutoPlaying())
}

func TestReplay_NextAdvancesCursor(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r, ok := updated.(tui.Replay)
	require.True(t, ok)

	assert.Equal(t, 1, r.Cursor())
}

func TestReplay_PrevAtZeroStays(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	assert.Equal(t, 0, updated.(tui.Replay).Cursor())
}

func TestReplay_NextAtEndStays(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r := u2.(tui.Replay)
	require.Equal(t, 2, r.Cursor())

	u3, _ := r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	assert.Equal(t, 2, u3.(tui.Replay).Cursor())
}

func TestReplay_NextTopicJumps(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	assert.Equal(t, 2, updated.(tui.Replay).Cursor())
}

func TestReplay_PrevTopicFromMidTopicJumpsToStart(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r := u1.(tui.Replay)
	require.Equal(t, 1, r.Cursor())

	u2, _ := r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})

	assert.Equal(t, 0, u2.(tui.Replay).Cursor())
}

func TestReplay_ToggleThinking(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	assert.True(t, u.(tui.Replay).ThinkingVisible())
}

func TestReplay_SpaceStartsAutoPlay(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	assert.True(t, u.(tui.Replay).AutoPlaying())
	assert.NotNil(t, cmd)
}

func TestReplay_SpaceAgainStopsAutoPlay(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	on, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	off, _ := on.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeySpace})

	assert.False(t, off.(tui.Replay).AutoPlaying())
}

func TestReplay_TickAdvancesCursorWhenPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	on, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	after, cmd := on.(tui.Replay).Update(tui.TickMsg{})

	assert.Equal(t, 1, after.(tui.Replay).Cursor())
	assert.NotNil(t, cmd)
}

func TestReplay_TickIgnoredWhenNotPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	after, cmd := m.Update(tui.TickMsg{})

	assert.Equal(t, 0, after.(tui.Replay).Cursor())
	assert.Nil(t, cmd)
}

func TestReplay_TickAtEndStopsPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	on, _ := u2.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeySpace})

	after, cmd := on.(tui.Replay).Update(tui.TickMsg{})

	assert.False(t, after.(tui.Replay).AutoPlaying())
	assert.Nil(t, cmd)
}

func TestReplay_SpeedUpClampsTo9(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	r1 := u1.(tui.Replay)

	for i := 0; i < 20; i++ {
		u2, _ := r1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
		r1 = u2.(tui.Replay)
	}

	assert.Equal(t, 9, r1.Speed())
}

func TestReplay_SpeedDownClampsTo1(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	r1 := u1.(tui.Replay)

	for i := 0; i < 20; i++ {
		u2, _ := r1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
		r1 = u2.(tui.Replay)
	}

	assert.Equal(t, 1, r1.Speed())
}

func TestReplay_SpeedKeysNoOpWhenNotAutoplaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	require.False(t, m.AutoPlaying())

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})

	assert.Equal(t, 5, u.(tui.Replay).Speed())
}

func TestReplay_ThinkingIndicatorInHeader(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	m = m.SetSize(120, 40).(tui.Replay)

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	out := u.View()
	assert.Contains(t, out, "thinking")
}

func TestReplay_EnterOnToolResultTurnExpands(t *testing.T) {
	sess := &parser.Session{
		ID: "s2",
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Content: "run ls"},
			{
				Role:        parser.RoleAssistant,
				ToolCalls:   []parser.ToolCall{{ID: "t1", Name: "Bash", Input: []byte(`{"command":"ls"}`)}},
				ToolResults: []parser.ToolResult{{ID: "t1", Content: "a\nb"}},
			},
		},
	}
	ts := []topics.Topic{{Label: "T", TurnIndices: []int{0, 1}}}
	m := tui.NewReplay(sess, ts)
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}) // cursor=1

	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, u2.(tui.Replay).ToolExpanded())
}

func TestReplay_EscWhileExpandedCollapses(t *testing.T) {
	sess := &parser.Session{
		Turns: []parser.Turn{
			{
				Role:        parser.RoleAssistant,
				ToolResults: []parser.ToolResult{{ID: "t1", Content: "x"}},
			},
		},
	}
	m := tui.NewReplay(sess, []topics.Topic{{TurnIndices: []int{0}}})
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, u1.(tui.Replay).ToolExpanded())

	u2, cmd := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, u2.(tui.Replay).ToolExpanded())
	assert.Nil(t, cmd)
}

func TestReplay_EscWhileCollapsedEmitsReturn(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.ReturnToOverviewMsg)
	assert.True(t, ok)
}

func TestReplay_View_SidebarVisibleAtWideWidth(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	m = m.SetSize(120, 40).(tui.Replay)

	out := m.View()

	assert.Contains(t, out, "Greet")
}

func TestReplay_View_NarrowHidesSidebar(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics())
	m = m.SetSize(60, 20).(tui.Replay)

	out := m.View()

	assert.Contains(t, out, "Greet")
	assert.Contains(t, out, "Replay")
}

func TestReplay_View_ExpandedShowsOnlyViewport(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{
		Role:        parser.RoleAssistant,
		ToolResults: []parser.ToolResult{{ID: "t1", Content: "EXPANDED_MARKER"}},
	}}}
	m := tui.NewReplay(sess, []topics.Topic{{TurnIndices: []int{0}}})
	m = m.SetSize(120, 40).(tui.Replay)
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	out := u.(tui.Replay).View()

	assert.Contains(t, out, "EXPANDED_MARKER")
	assert.NotContains(t, out, "Greet")
}
