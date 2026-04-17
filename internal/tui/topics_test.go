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
