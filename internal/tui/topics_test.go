package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/topics"
	"github.com/justincordova/agentlens/internal/tui"
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
	// Topics are sorted latest-first, so the last clustered topic renders first.
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	out := o.View()
	assert.Contains(t, out, tops[len(tops)-1].Label)
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

func TestTokenBar_HalfFull(t *testing.T) {
	got := tui.TokenBar(50, 100, 8)
	assert.Equal(t, "████░░░░", got)
}

func TestTokenBar_Full(t *testing.T) {
	got := tui.TokenBar(100, 100, 8)
	assert.Equal(t, "████████", got)
}

func TestTokenBar_Empty(t *testing.T) {
	got := tui.TokenBar(0, 100, 8)
	assert.Equal(t, "░░░░░░░░", got)
}

func TestTokenBar_ZeroSessionTotal_RendersEmpty(t *testing.T) {
	got := tui.TokenBar(0, 0, 8)
	assert.Equal(t, "░░░░░░░░", got)
}

func TestOverview_View_ContainsTokenBar(t *testing.T) {
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	out := o.View()
	assert.Contains(t, out, "█")
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
	// Topics are sorted latest-first, so cursor=0 is the later topic.
	s, tops := demoSessionAndTopics()
	o := tui.NewOverview(s, tops)
	next, _ := o.Update(tea.KeyMsg{Type: tea.KeyEnter})
	o = next.(tui.Overview)
	out := o.View()
	assert.Contains(t, out, "switching to database setup now")
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

func TestOverview_Topics_SortedLatestFirst(t *testing.T) {
	// Arrange — two topics: earlier (turns 1–2), later (turns 3–4).
	s, tops := demoSessionAndTopics()
	require.Len(t, tops, 2)
	o := tui.NewOverview(s, tops)

	// Act
	out := o.View()

	// Assert — the later topic's turn range appears before the earlier one's.
	laterRange := "turns 3–4"
	earlierRange := "turns 1–2"
	laterIdx := strings.Index(out, laterRange)
	earlierIdx := strings.Index(out, earlierRange)
	require.Greater(t, laterIdx, -1, "later turn range should be rendered")
	require.Greater(t, earlierIdx, -1, "earlier turn range should be rendered")
	assert.Less(t, laterIdx, earlierIdx, "latest topic must render before earlier one")
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
	assert.NotContains(t, out, "turns 1–2", "topic cards should be hidden while stats are toggled on")
	assert.NotContains(t, out, "turns 3–4", "topic cards should be hidden while stats are toggled on")
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
