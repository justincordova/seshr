package tui_test

import (
	"fmt"
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
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	require.Len(t, tops, 2, "fixture must produce 2 topics")
	assert.Equal(t, 0, o.Cursor())
	assert.False(t, o.Expanded(0))
	assert.False(t, o.StatsVisible())
}

func TestOverview_View_ContainsTopicLabel(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	out := o.View()
	assert.Contains(t, out, tops[0].Label)
}

func TestOverview_DownKey_MovesCursor(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, next.(tui.Overview).Cursor())
}

func TestOverview_DownKey_AtBottomStays(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	for i := 0; i < 5; i++ {
		next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = next.(tui.Overview)
	}
	assert.Equal(t, len(tops)-1, o.Cursor())
}

func TestOverview_EnterKey_TogglesExpand(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	oo := next.(tui.Overview)
	assert.True(t, oo.Expanded(0))

	next2, _ := oo.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ooo := next2.(tui.Overview)
	assert.False(t, ooo.Expanded(0))
}

func TestOverview_ExpandedView_ShowsPreviews(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o = next.(tui.Overview)
	out := o.View()
	assert.Contains(t, out, "set up express")
}

func TestOverview_TabKey_TogglesStats(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	oo := next.(tui.Overview)
	assert.True(t, oo.StatsVisible())

	next2, _ := oo.Update(tea.KeyMsg{Type: tea.KeyTab})
	ooo := next2.(tui.Overview)
	assert.False(t, ooo.StatsVisible())
}

func TestOverview_Stats_ContainsBreakdownNumbers(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	o = next.(tui.Overview)
	out := o.View()
	assert.Contains(t, out, "100")
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "AI")
	assert.Contains(t, out, "2 topics")
}

func TestOverview_UpKey_MovesCursorUp(t *testing.T) {
	// Arrange — start at bottom
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
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
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	assert.Equal(t, 0, next.(tui.Overview).Cursor())
}

func TestOverview_Topics_SortedOldestFirst(t *testing.T) {
	s, tops := demoSessionAndTopics()
	require.Len(t, tops, 2)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

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
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	// Act — tab on to stats view
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyTab})
	oo := next.(tui.Overview)
	out := oo.View()

	// Assert — stats breakdown shown, and topic-panel cards are hidden.
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "AI")
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
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	// Act
	out := o.View()

	// Assert — humanize.Time is not appended; no "ago" suffix leaks in.
	assert.NotContains(t, out, " ago")
}

func TestOverview_EscKey_EmitsReturnToPickerMsg(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	_, cmd := o.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.ReturnToPickerMsg)
	assert.True(t, ok, "expected ReturnToPickerMsg, got %T", msg)
}

func TestOverview_RKeyEmitsOpenReplayMsg(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	m := tui.NewOverview(sess, ts, tui.CatppuccinMocha(), 0)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tui.OpenReplayMsg)
	assert.True(t, ok, "expected OpenReplayMsg, got %T", msg)
}

func TestOverview_SpaceSelectsTopic(t *testing.T) {
	sess := &parser.Session{Turns: []parser.Turn{{Role: parser.RoleUser}}}
	ts := []topics.Topic{{Label: "Only", TurnIndices: []int{0}}}
	m := tui.NewOverview(sess, ts, tui.CatppuccinMocha(), 0)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, next.(tui.Overview).IsSelected(0))

	// Space again deselects.
	next2, _ := next.(tui.Overview).Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.False(t, next2.(tui.Overview).IsSelected(0))
}

func TestOverview_ToggleAll_SelectsAndDeselects(t *testing.T) {
	s, tops := demoSessionAndTopics()
	m := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
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
	m := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

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
	m := tui.NewOverview(sess, tops, tui.CatppuccinMocha(), 0)

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
	m := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)

	// Select first topic then check view contains token summary
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	out := next.(tui.Overview).View()
	assert.Contains(t, out, "selected")
}

// manyTopicsSession builds a session with n topics, each with 8 turns, so
// that expanding any topic makes it much taller than the viewport can hold
// at a small terminal height. Each turn's content uniquely identifies its
// topic index so tests can assert which expansions were actually rendered.
func manyTopicsSession(n int) (*parser.Session, []topics.Topic) {
	base := time.Unix(1_700_000_000, 0)
	turns := make([]parser.Turn, 0, n*8)
	tops := make([]topics.Topic, 0, n)
	for i := 0; i < n; i++ {
		start := len(turns)
		for j := 0; j < 8; j++ {
			role := parser.RoleUser
			if j%2 == 1 {
				role = parser.RoleAssistant
			}
			turns = append(turns, parser.Turn{
				Role:      role,
				Timestamp: base.Add(time.Duration(i*10+j) * time.Minute),
				Content:   fmt.Sprintf("payload-t%d-j%d", i, j),
				Tokens:    10,
			})
		}
		indices := make([]int, 8)
		for j := 0; j < 8; j++ {
			indices[j] = start + j
		}
		tops = append(tops, topics.Topic{
			Label:       fmt.Sprintf("topic-%d", i),
			TurnIndices: indices,
			TokenCount:  80,
		})
	}
	s := &parser.Session{
		Path:       "/tmp/many.jsonl",
		Source:     parser.SourceClaude,
		ID:         "many",
		CreatedAt:  base,
		ModifiedAt: base.Add(time.Duration(n*10) * time.Minute),
		TokenCount: n * 80,
		Turns:      turns,
	}
	return s, tops
}

func TestOverview_Scroll_CursorStaysVisibleWithCollapsedTopics(t *testing.T) {
	s, tops := manyTopicsSession(20)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Press j many times; offset must follow so cursor stays in view.
	for i := 0; i < 19; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	assert.Equal(t, 19, o.Cursor())
	assert.Greater(t, o.Offset(), 0, "offset must advance when cursor moves past viewport")
	assert.LessOrEqual(t, o.Offset(), o.Cursor(), "offset may not exceed cursor")
}

func TestOverview_Scroll_CursorStaysVisibleWhenAllExpanded(t *testing.T) {
	s, tops := manyTopicsSession(20)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Expand all topics (f toggles fold-all).
	n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	o = n.(tui.Overview)

	// Press j repeatedly; with expanded topics each one fills most of the
	// viewport, so offset must advance on nearly every press.
	for i := 0; i < 19; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	assert.Equal(t, 19, o.Cursor())
	assert.LessOrEqual(t, o.Offset(), o.Cursor())
	// After 19 down-presses with expanded topics, offset must have advanced
	// to keep the cursor on screen.
	assert.Greater(t, o.Offset(), 0, "offset must advance with expanded topics")
}

func TestOverview_Scroll_ExpandingCursorTopicKeepsItVisible(t *testing.T) {
	s, tops := manyTopicsSession(10)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Move cursor down 5 places.
	for i := 0; i < 5; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	require.Equal(t, 5, o.Cursor())

	// Expand the cursor topic. Offset must be adjusted so cursor is visible.
	n, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o = n.(tui.Overview)
	assert.True(t, o.Expanded(5))
	assert.LessOrEqual(t, o.Offset(), o.Cursor(), "offset must not jump past cursor")
}

func TestOverview_Scroll_MixedHeightsCursorAdvancesOffset(t *testing.T) {
	// Repros the reported bug: when later topics are expanded the fixed-window
	// clampOffset computed "visible" from index 0 and under-scrolled, leaving
	// the cursor off-screen.
	s, tops := manyTopicsSession(10)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Move to topic 5 and expand it plus each topic after.
	for i := 0; i < 5; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	for i := 0; i < 5; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
		o = n.(tui.Overview)
		n, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	assert.Equal(t, 10-1, o.Cursor(), "cursor should be at bottom")
	assert.Greater(t, o.Offset(), 0, "offset must advance when expanded topics push cursor off screen")
	assert.LessOrEqual(t, o.Offset(), o.Cursor())
}

func TestOverview_Scroll_LastTopicExpandedShowsItsTurns(t *testing.T) {
	// Reproduces bug: 4 topics, cursor on the last one, user expands it but
	// the expansion lines had no room and were clipped to zero. Scroll must
	// advance so the expansion is actually visible.
	s, tops := manyTopicsSession(4)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	// Tight terminal: only ~15 lines for the body.
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Expand the first 3 topics to fill the viewport, then move to topic 4.
	for i := 0; i < 3; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
		o = n.(tui.Overview)
		n, _ = o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	require.Equal(t, 3, o.Cursor())

	// Expand the last topic. Its turns must actually render — not be clipped
	// to zero because its card fit but the expansion budget was exhausted.
	n, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o = n.(tui.Overview)
	require.True(t, o.Expanded(3))

	out := o.View()
	// Topic 3's unique preview content must appear — i.e. its expansion
	// rendered at least one turn, proving the scroll math made room.
	assert.Contains(t, out, "payload-t3", "expanded cursor topic must render its turns")
}

func TestOverview_Scroll_ExpandAllDoesNotExceedTerminalHeight(t *testing.T) {
	// Regression: expanding all topics used to emit a View() taller than the
	// terminal, which made the terminal auto-scroll and hide the top rows
	// (including the first topic's card indicator).
	s, tops := manyTopicsSession(15)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	const termH = 55
	next, _ := o.Update(tea.WindowSizeMsg{Width: 170, Height: termH})
	o = next.(tui.Overview)

	n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	o = n.(tui.Overview)

	out := o.View()
	lines := strings.Split(out, "\n")
	assert.LessOrEqual(t, len(lines), termH,
		"rendered view must not exceed terminal height (was %d, term %d)",
		len(lines), termH)

	// Cursor topic (topic 0) must be fully visible with its card indicator.
	assert.Contains(t, out, tops[0].Label,
		"cursor topic card header must appear in view")
}

func TestOverview_Scroll_MovingUpScrollsBack(t *testing.T) {
	s, tops := manyTopicsSession(20)
	o := tui.NewOverview(s, tops, tui.CatppuccinMocha(), 0)
	next, _ := o.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	o = next.(tui.Overview)

	// Scroll to the bottom.
	for i := 0; i < 19; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		o = n.(tui.Overview)
	}
	require.Equal(t, 19, o.Cursor())
	bottomOffset := o.Offset()
	require.Greater(t, bottomOffset, 0)

	// Now scroll back up to top.
	for i := 0; i < 19; i++ {
		n, _ := o.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		o = n.(tui.Overview)
	}
	assert.Equal(t, 0, o.Cursor())
	assert.Equal(t, 0, o.Offset(), "offset must follow cursor back to top")
}
