package topics_test

import (
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/stretchr/testify/assert"
)

func turnAt(ts time.Time, role session.Role, content string) session.Turn {
	return session.Turn{Role: role, Timestamp: ts, Content: content}
}

func TestTimeGapScore_BelowThreshold_Zero(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "hi")
	cur := turnAt(time.Unix(60, 0), session.RoleAssistant, "hey")
	got := topics.TimeGapScore(prev, cur, topics.DefaultOptions())
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestTimeGapScore_ExactlyThreshold_Zero(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "hi")
	cur := turnAt(time.Unix(180, 0), session.RoleAssistant, "hey")
	got := topics.TimeGapScore(prev, cur, topics.DefaultOptions())
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestTimeGapScore_OverThreshold_ReturnsWeight(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "hi")
	cur := turnAt(time.Unix(4*60, 0), session.RoleAssistant, "hey")
	got := topics.TimeGapScore(prev, cur, topics.DefaultOptions())
	assert.InDelta(t, 0.45, got, 0.001)
}

func TestExplicitMarkerScore_UserMarkerPhrase_ReturnsWeight(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleAssistant, "done")
	cur := turnAt(time.Unix(1, 0), session.RoleUser, "actually, can you switch gears to something else")
	got := topics.ExplicitMarkerScore(prev, cur)
	assert.InDelta(t, 0.15, got, 0.001)
}

func TestExplicitMarkerScore_AssistantSide_Zero(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "what?")
	cur := turnAt(time.Unix(1, 0), session.RoleAssistant, "let's move on to part 2")
	got := topics.ExplicitMarkerScore(prev, cur)
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestExplicitMarkerScore_NoMarker_Zero(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleAssistant, "done")
	cur := turnAt(time.Unix(1, 0), session.RoleUser, "add another test case")
	got := topics.ExplicitMarkerScore(prev, cur)
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestFileShiftScore_SameFiles_Zero(t *testing.T) {
	got := topics.FileShiftScore([]string{"/a.go"}, []string{"/a.go"}, topics.DefaultOptions())
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestFileShiftScore_BelowJaccardThreshold_ReturnsWeight(t *testing.T) {
	got := topics.FileShiftScore([]string{"/a.go", "/b.go"}, []string{"/c.go", "/d.go"}, topics.DefaultOptions())
	assert.InDelta(t, 0.25, got, 0.001)
}

func TestFileShiftScore_BothEmpty_Zero(t *testing.T) {
	got := topics.FileShiftScore(nil, nil, topics.DefaultOptions())
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestKeywordScore_HighOverlap_Zero(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "auth middleware jwt token")
	cur := turnAt(time.Unix(1, 0), session.RoleAssistant, "jwt token auth middleware works")
	got := topics.KeywordScore(prev, cur, topics.DefaultOptions())
	assert.InDelta(t, 0.0, got, 0.001)
}

func TestKeywordScore_LowOverlap_ReturnsWeight(t *testing.T) {
	prev := turnAt(time.Unix(0, 0), session.RoleUser, "express server route middleware jwt")
	cur := turnAt(time.Unix(1, 0), session.RoleUser, "recipe pancake breakfast syrup butter")
	got := topics.KeywordScore(prev, cur, topics.DefaultOptions())
	assert.InDelta(t, 0.15, got, 0.001)
}
