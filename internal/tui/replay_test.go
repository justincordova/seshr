package tui_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSession() *session.Session {
	return &session.Session{
		ID: "s1",
		Turns: []session.Turn{
			{Role: session.RoleUser, Content: "hello", Timestamp: time.Unix(100, 0)},
			{Role: session.RoleAssistant, Content: "hi", Timestamp: time.Unix(110, 0)},
			{Role: session.RoleUser, Content: "next", Timestamp: time.Unix(120, 0)},
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
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	assert.Equal(t, 0, m.Cursor())
	assert.False(t, m.ThinkingVisible())
	assert.False(t, m.AutoPlaying())
}

func TestReplay_VimGotoFirstTurn(t *testing.T) {
	// Arrange — advance cursor.
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(tui.Replay)
	require.Equal(t, 1, m.Cursor())

	// Act
	next2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})

	// Assert
	assert.Equal(t, 0, next2.(tui.Replay).Cursor())
}

func TestReplay_VimGotoLastTurn(t *testing.T) {
	// Arrange
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})

	// Assert
	assert.Equal(t, 2, next.(tui.Replay).Cursor())
}

func TestReplay_MainPanelScrollsLongTurn(t *testing.T) {
	// Arrange: a turn whose body is far taller than the viewport. The main
	// viewport's content must be set from Update (View works on a copy), or
	// every scroll key is a silent no-op.
	long := ""
	for i := 0; i < 200; i++ {
		long += "line\n\n"
	}
	sess := &session.Session{
		ID: "s1",
		Turns: []session.Turn{
			{Role: session.RoleUser, Content: long, Timestamp: time.Unix(100, 0)},
		},
	}
	m := tui.NewReplay(sess, nil, tui.CatppuccinMocha())
	m = m.SetSize(100, 24).(tui.Replay)
	require.Equal(t, 0, m.MainScrollOffset())

	// Act
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Assert
	assert.Greater(t, next.(tui.Replay).MainScrollOffset(), 0,
		"j must scroll the main panel on a long turn")
}

func TestReplay_VimPageKeysNoCrash(t *testing.T) {
	// Arrange
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	// Act — both should be safe on a small fixture.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
}

func TestReplay_NextAdvancesCursor(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r, ok := updated.(tui.Replay)
	require.True(t, ok)

	assert.Equal(t, 1, r.Cursor())
}

func TestReplay_PrevAtZeroStays(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	assert.Equal(t, 0, updated.(tui.Replay).Cursor())
}

func TestReplay_NextAtEndStays(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r := u2.(tui.Replay)
	require.Equal(t, 2, r.Cursor())

	u3, _ := r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	assert.Equal(t, 2, u3.(tui.Replay).Cursor())
}

func TestReplay_NextTopicJumps(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	assert.Equal(t, 2, updated.(tui.Replay).Cursor())
}

func TestReplay_PrevTopicFromMidTopicJumpsToStart(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	r := u1.(tui.Replay)
	require.Equal(t, 1, r.Cursor())

	u2, _ := r.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})

	assert.Equal(t, 0, u2.(tui.Replay).Cursor())
}

func TestReplay_NextTopicResetsScroll(t *testing.T) {
	// Arrange: first topic's turn is tall enough to scroll; second topic is a
	// separate turn to jump to.
	long := ""
	for i := 0; i < 200; i++ {
		long += "line\n\n"
	}
	sess := &session.Session{
		ID: "s1",
		Turns: []session.Turn{
			{Role: session.RoleUser, Content: long, Timestamp: time.Unix(100, 0)},
			{Role: session.RoleUser, Content: "second topic", Timestamp: time.Unix(110, 0)},
		},
	}
	ts := []topics.Topic{
		{Label: "Long", TurnIndices: []int{0}},
		{Label: "Next", TurnIndices: []int{1}},
	}
	m := tui.NewReplay(sess, ts, tui.CatppuccinMocha())
	m = m.SetSize(100, 24).(tui.Replay)
	scrolled, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = scrolled.(tui.Replay)
	require.Greater(t, m.MainScrollOffset(), 0)

	// Act: jump to the next topic.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	// Assert: landed on the new topic's turn, scrolled to the top.
	r := next.(tui.Replay)
	assert.Equal(t, 1, r.Cursor())
	assert.Equal(t, 0, r.MainScrollOffset(), "topic jump must reset scroll to top of new turn")
}

func TestReplay_NextTopicFromUncategorizedTurnGoesToNextTopic(t *testing.T) {
	// A system turn between two topics belongs to no topic; pressing ] must
	// advance to the next topic, not jump to the end of the session.
	sess := &session.Session{
		ID: "s1",
		Turns: []session.Turn{
			{Role: session.RoleUser, Content: "a", Timestamp: time.Unix(100, 0)},
			{Role: session.RoleSystem, Content: "sys", Timestamp: time.Unix(105, 0)},
			{Role: session.RoleUser, Content: "b", Timestamp: time.Unix(110, 0)},
			{Role: session.RoleUser, Content: "c", Timestamp: time.Unix(120, 0)},
		},
	}
	ts := []topics.Topic{
		{Label: "First", TurnIndices: []int{0}},
		{Label: "Second", TurnIndices: []int{2}},
		{Label: "Third", TurnIndices: []int{3}},
	}
	m := tui.NewReplay(sess, ts, tui.CatppuccinMocha())
	// Move cursor onto the uncategorized system turn (index 1).
	step, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = step.(tui.Replay)
	require.Equal(t, 1, m.Cursor())

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	assert.Equal(t, 2, next.(tui.Replay).Cursor(),
		"] from an uncategorized turn must land on the next topic, not the session end")
}

func TestReplay_ToggleThinking(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	assert.True(t, u.(tui.Replay).ThinkingVisible())
}

func TestReplay_SpaceStartsAutoPlay(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	assert.True(t, u.(tui.Replay).AutoPlaying())
	assert.NotNil(t, cmd)
}

func TestReplay_SpaceAgainStopsAutoPlay(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	on, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	off, _ := on.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeySpace})

	assert.False(t, off.(tui.Replay).AutoPlaying())
}

func TestReplay_TickAdvancesCursorWhenPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	on, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})

	r := on.(tui.Replay)
	after, cmd := r.Update(tui.TickMsg{Gen: r.PlayGen()})

	assert.Equal(t, 1, after.(tui.Replay).Cursor())
	assert.NotNil(t, cmd)
}

func TestReplay_TickIgnoredWhenNotPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	after, cmd := m.Update(tui.TickMsg{})

	assert.Equal(t, 0, after.(tui.Replay).Cursor())
	assert.Nil(t, cmd)
}

func TestReplay_TickAtEndStopsPlaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	on, _ := u2.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeySpace})

	r := on.(tui.Replay)
	after, cmd := r.Update(tui.TickMsg{Gen: r.PlayGen()})

	assert.False(t, after.(tui.Replay).AutoPlaying())
	assert.Nil(t, cmd)
}

func TestReplay_StaleTickAfterPauseResumeIgnored(t *testing.T) {
	// Reproduces the autoplay ticker-fork bug: tea.Tick cannot be cancelled,
	// so a tick scheduled before a pause must be ignored after resume,
	// otherwise it forks a second self-perpetuating tick chain.
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	// Start playing (generation N) and remember its tick generation.
	play, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	r := play.(tui.Replay)
	staleGen := r.PlayGen()

	// Pause, then resume — resume re-arms a new generation.
	paused, _ := r.Update(tea.KeyMsg{Type: tea.KeySpace})
	resumed, _ := paused.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeySpace})
	rr := resumed.(tui.Replay)
	require.True(t, rr.AutoPlaying())
	require.NotEqual(t, staleGen, rr.PlayGen())

	// Deliver the stale tick from the pre-pause chain. It must be a no-op:
	// no cursor advance and no new ticker spawned.
	after, cmd := rr.Update(tui.TickMsg{Gen: staleGen})
	assert.Equal(t, 0, after.(tui.Replay).Cursor())
	assert.Nil(t, cmd)
}

func TestReplay_SpeedUpClampsTo9(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	r1 := u1.(tui.Replay)

	for i := 0; i < 20; i++ {
		u2, _ := r1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
		r1 = u2.(tui.Replay)
	}

	assert.Equal(t, 9, r1.Speed())
}

func TestReplay_SpeedDownClampsTo1(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	r1 := u1.(tui.Replay)

	for i := 0; i < 20; i++ {
		u2, _ := r1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}})
		r1 = u2.(tui.Replay)
	}

	assert.Equal(t, 1, r1.Speed())
}

func TestReplay_SpeedKeysNoOpWhenNotAutoplaying(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	require.False(t, m.AutoPlaying())

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})

	assert.Equal(t, 5, u.(tui.Replay).Speed())
}

func TestReplay_ThinkingIndicatorInHeader(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	m = m.SetSize(120, 40).(tui.Replay)

	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	out := u.View()
	assert.Contains(t, out, "thinking")
}

func TestReplay_EnterOnToolResultTurnExpands(t *testing.T) {
	sess := &session.Session{
		ID: "s2",
		Turns: []session.Turn{
			{Role: session.RoleUser, Content: "run ls"},
			{
				Role:        session.RoleAssistant,
				ToolCalls:   []session.ToolCall{{ID: "t1", Name: "Bash", Input: []byte(`{"command":"ls"}`)}},
				ToolResults: []session.ToolResult{{ID: "t1", Content: "a\nb"}},
			},
		},
	}
	ts := []topics.Topic{{Label: "T", TurnIndices: []int{0, 1}}}
	m := tui.NewReplay(sess, ts, tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}) // cursor=1

	u2, _ := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, u2.(tui.Replay).ToolExpanded())
}

func TestReplay_EscWhileExpandedCollapses(t *testing.T) {
	sess := &session.Session{
		Turns: []session.Turn{
			{
				Role:        session.RoleAssistant,
				ToolResults: []session.ToolResult{{ID: "t1", Content: "x"}},
			},
		},
	}
	m := tui.NewReplay(sess, []topics.Topic{{TurnIndices: []int{0}}}, tui.CatppuccinMocha())
	u1, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, u1.(tui.Replay).ToolExpanded())

	u2, cmd := u1.(tui.Replay).Update(tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, u2.(tui.Replay).ToolExpanded())
	assert.Nil(t, cmd)
}

func TestReplay_EscWhileCollapsedEmitsReturn(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.ReturnToOverviewMsg)
	assert.True(t, ok)
}

func TestReplay_View_SidebarVisibleAtWideWidth(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	m = m.SetSize(120, 40).(tui.Replay)

	out := m.View()

	assert.Contains(t, out, "Greet")
}

func TestReplay_View_NarrowHidesSidebar(t *testing.T) {
	m := tui.NewReplay(sampleSession(), sampleTopics(), tui.CatppuccinMocha())
	m = m.SetSize(60, 20).(tui.Replay)

	out := m.View()

	assert.Contains(t, out, "Greet")
	assert.Contains(t, out, "Replay")
}

func TestReplay_View_ExpandedShowsOnlyViewport(t *testing.T) {
	sess := &session.Session{Turns: []session.Turn{{
		Role:        session.RoleAssistant,
		ToolResults: []session.ToolResult{{ID: "t1", Content: "EXPANDED_MARKER"}},
	}}}
	m := tui.NewReplay(sess, []topics.Topic{{TurnIndices: []int{0}}}, tui.CatppuccinMocha())
	m = m.SetSize(120, 40).(tui.Replay)
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	out := u.(tui.Replay).View()

	assert.Contains(t, out, "EXPANDED_MARKER")
	assert.NotContains(t, out, "Greet")
}
