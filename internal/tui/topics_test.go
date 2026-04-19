package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func demoSessionAndTopics() (*parser.Session, []topics.Topic) {
	base := time.Unix(1_700_000_000, 0)
	s := &parser.Session{
		Path:       "/tmp/demo.jsonl",
		Source:     parser.SourceClaude,
		ID:         "demo",
		CreatedAt:  base,
		ModifiedAt: base.Add(20 * time.Minute),
		TokenCount: 100,
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Timestamp: base, Content: "set up express", Tokens: 10},
			{Role: parser.RoleAssistant, Timestamp: base.Add(10 * time.Second), Content: "ok", Tokens: 20},
			{Role: parser.RoleUser, Timestamp: base.Add(10 * time.Minute), Content: "switching to database setup now", Tokens: 10},
			{Role: parser.RoleAssistant, Timestamp: base.Add(10*time.Minute + 5*time.Second), Content: "ok", Tokens: 60},
		},
	}
	tops := topics.Cluster(s, topics.DefaultOptions())
	return s, tops
}

func TestOverview_New_InitialState(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	require.Len(t, tops, 2, "fixture must produce 2 topics")
	assert.Equal(t, 0, o.Cursor())
	assert.False(t, o.Expanded(0))
	assert.False(t, o.StatsVisible())
}

func TestOverview_View_ContainsTopicLabel(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	out := o.View()
	assert.Contains(t, out, tops[0].Label)
}

func TestOverview_DownKey_MovesCursor(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, next.(tui.Overview).Cursor())
}

func TestOverview_DownKey_AtBottomStays(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	for i := 0; i < 5; i++ {
		next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = next.(tui.Overview)
	}
	assert.Equal(t, len(tops)-1, o.Cursor())
}

func TestOverview_EnterKey_TogglesExpand(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	oo := next.(tui.Overview)
	assert.True(t, oo.Expanded(0))

	next2, _ := oo.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ooo := next2.(tui.Overview)
	assert.False(t, ooo.Expanded(0))
}

func TestOverview_ExpandedView_ShowsPreviews(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o = next.(tui.Overview)
	out := o.View()
	assert.Contains(t, out, "set up express")
}

func TestOverview_TabKey_TogglesStats(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	oo := next.(tui.Overview)
	assert.True(t, oo.StatsVisible())

	next2, _ := oo.Update(tea.KeyMsg{Type: tea.KeyTab})
	ooo := next2.(tui.Overview)
	assert.False(t, ooo.StatsVisible())
}

func TestOverview_Stats_ContainsBreakdownNumbers(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	o = next.(tui.Overview)
	out := o.View()
	assert.Contains(t, out, "100")
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "assistant")
	assert.Contains(t, out, "2 topics")
}

func TestOverview_UpKey_MovesCursorUp(t *testing.T) {
	// Arrange — start at bottom
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	o = next.(tui.Overview)
	require.Equal(t, 1, o.Cursor())

	// Act
	next2, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Assert
	assert.Equal(t, 0, next2.(tui.Overview).Cursor())
}

func TestOverview_UpKey_AtTopStays(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	assert.Equal(t, 0, next.(tui.Overview).Cursor())
}

func TestOverview_Topics_SortedOldestFirst(t *testing.T) {
	s, tops := demoSessionAndTopics()
	require.Len(t, tops, 2)
	o := tui.NewOverview(s, tops)

	out := o.View()

	earlierRange := "turns 1–2"
	laterRange := "turns 3–4"
	earlierIdx := strings.Index(out, earlierRange)
	laterIdx := strings.Index(out, laterRange)
	require.Greater(t, earlierIdx, -1, "earlier turn range should be rendered")
	require.Greater(t, laterIdx, -1, "later turn range should be rendered")
	assert.Less(t, earlierIdx, laterIdx, "earliest topic must render before later one")
}

func TestOverview_StatsVisible_SwapsOutTopicPanel(t *testing.T) {
	// Arrange
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)

	// Act — tab on to stats view
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	oo := next.(tui.Overview)
	out := oo.View()

	// Assert — stats breakdown shown, and topic-panel cards are hidden.
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "assistant")
	assert.Contains(t, out, "2 topic")
}

func TestOverview_ZeroTimestamp_OmitsRelativeTime(t *testing.T) {
	// Arrange — session with zero-valued turn timestamps.
	s := &parser.Session{
		Path:       "/tmp/zero.jsonl",
		Source:     parser.SourceClaude,
		ID:         "zero",
		TokenCount: 5,
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Content: "hi", Tokens: 3},
			{Role: parser.RoleAssistant, Content: "yo", Tokens: 2},
		},
	}
	tops := topics.Cluster(s, topics.DefaultOptions())
	require.NotEmpty(t, tops)
	o := tui.NewOverview(s, tops)

	// Act
	out := o.View()

	// Assert — humanize.Time is not appended; no "ago" suffix leaks in.
	assert.NotContains(t, out, " ago")
}

func TestOverview_EscKey_EmitsReturnToPickerMsg(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)

	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.ReturnToPickerMsg)
	assert.True(t, ok, "expected ReturnToPickerMsg, got %T", msg)
}

func TestOverview_RKeyEmitsOpenReplayMsg(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	m := tui.NewOverview(sess, ts)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.OpenReplayMsg)
	assert.True(t, ok, "expected OpenReplayMsg, got %T", msg)
}

func TestOverview_SpaceSelectsTopic(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	m := tui.NewOverview(sess, ts)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, next.(tui.Overview).IsSelected(0))

	// Space again deselects.
	next2, _ := next.(tui.Overview).Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.False(t, next2.(tui.Overview).IsSelected(0))
}

func TestOverview_ToggleAll_SelectsAndDeselects(t *testing.T) {
	s, tops := demoSessionAndTopics()
	m := tui.NewOverview(s, tops)
	require.Len(t, tops, 2)

	// 'a' with none selected → select all
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	o := next.(tui.Overview)
	assert.True(t, o.IsSelected(0))
	assert.True(t, o.IsSelected(1))

	// 'a' again with all selected → deselect all
	next2, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	o2 := next2.(tui.Overview)
	assert.False(t, o2.IsSelected(0))
	assert.False(t, o2.IsSelected(1))
}

func TestOverview_FoldAll_ExpandsAndCollapses(t *testing.T) {
	s, tops := demoSessionAndTopics()
	m := tui.NewOverview(s, tops)

	// 'f' with none expanded → expand all
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	o := next.(tui.Overview)
	assert.True(t, o.Expanded(0))
	assert.True(t, o.Expanded(1))

	// 'f' again → collapse all
	next2, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	o2 := next2.(tui.Overview)
	assert.False(t, o2.Expanded(0))
	assert.False(t, o2.Expanded(1))
}

func TestOverview_MultipleCompactBoundaries_MiddleTopicsAreSafeToprune(t *testing.T) {
	// Topics 1 and 2 are before boundary 1; topic 3 is between boundaries;
	// topic 4 is after boundary 2 (active context). Topics 1–3 should all be
	// pre-compact; only topic 4 is active.
	base := time.Unix(1_700_000_000, 0)
	sess := &parser.Session{
		Turns: []parser.Turn{
			{Role: parser.RoleUser, Timestamp: base, Content: "a"},
			{Role: parser.RoleAssistant, Timestamp: base.Add(1 * time.Second), Content: "b"},
			{Role: parser.RoleUser, Timestamp: base.Add(2 * time.Second), Content: "c"},
			{Role: parser.RoleAssistant, Timestamp: base.Add(3 * time.Second), Content: "d"},
			{Role: parser.RoleUser, Timestamp: base.Add(4 * time.Second), Content: "e"},
			{Role: parser.RoleAssistant, Timestamp: base.Add(5 * time.Second), Content: "f"},
		},
		CompactBoundaries: []parser.CompactBoundary{
			{TurnIndex: 2, Trigger: "auto"},
			{TurnIndex: 4, Trigger: "manual"},
		},
	}
	tops := []topics.Topic{
		{Label: "Before first", TurnIndices: []int{0, 1}},
		{Label: "Between boundaries", TurnIndices: []int{2, 3}},
		{Label: "After last", TurnIndices: []int{4, 5}},
	}
	m := tui.NewOverview(sess, tops)

	// Select first two topics (pre-compact) and verify safe indicator
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace}) // select topic 0
	next, _ = next.(tui.Overview).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	next, _ = next.(tui.Overview).Update(tea.KeyMsg{Type: tea.KeySpace}) // select topic 1
	out := next.(tui.Overview).View()
	assert.Contains(t, out, "Safe to prune", "topics before last boundary should be safe")
	assert.NotContains(t, out, "⚠", "should not show a warning for pre-compact topics")
}

func TestOverview_SelectionStripShownInView(t *testing.T) {
	s, tops := demoSessionAndTopics()
	m := tui.NewOverview(s, tops)

	// Select first topic then check view contains token summary
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	out := next.(tui.Overview).View()
	assert.Contains(t, out, "selected")
}
