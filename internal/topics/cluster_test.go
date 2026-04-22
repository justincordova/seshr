package topics_test

import (
	"context"
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCluster_NilSession_ReturnsNil(t *testing.T) {
	// Arrange / Act
	got := topics.Cluster(nil, topics.DefaultOptions())

	// Assert
	assert.Nil(t, got)
}

func TestTopic_ZeroValue_HasUsableDefaults(t *testing.T) {
	// Arrange / Act
	top := topics.Topic{}

	// Assert
	assert.Empty(t, top.Label)
	assert.Nil(t, top.TurnIndices)
	assert.Equal(t, 0, top.TokenCount)
	assert.Equal(t, 0, top.ToolCallCount)
	assert.Equal(t, time.Duration(0), top.Duration)
}

func TestDefaultOptions_MatchesSpecDefaults(t *testing.T) {
	// Arrange / Act
	opts := topics.DefaultOptions()

	// Assert — SPEC §5.1: 3-minute gap threshold
	assert.Equal(t, 3*time.Minute, opts.GapThreshold)
	// Jaccard threshold for file shift per SPEC §5.1
	assert.InDelta(t, 0.3, opts.FileJaccardThreshold, 0.001)
	// Keyword overlap threshold per SPEC §5.1
	assert.InDelta(t, 0.2, opts.KeywordOverlapThreshold, 0.001)
	// Boundary score threshold: empirical
	assert.Greater(t, opts.BoundaryThreshold, 0.0)
}

func makeSession(turns ...session.Turn) *session.Session {
	s := &session.Session{Turns: turns}
	for _, t := range turns {
		s.TokenCount += t.Tokens
	}
	return s
}

func userTurn(ts time.Time, content string, tokens int) session.Turn {
	return session.Turn{Role: session.RoleUser, Timestamp: ts, Content: content, Tokens: tokens}
}

func asstTurn(ts time.Time, content string, tokens int) session.Turn {
	return session.Turn{Role: session.RoleAssistant, Timestamp: ts, Content: content, Tokens: tokens}
}

func TestCluster_EmptySession_ReturnsNoTopics(t *testing.T) {
	got := topics.Cluster(&session.Session{}, topics.DefaultOptions())
	assert.Empty(t, got)
}

func TestCluster_NoBoundaries_ReturnsSingleTopic(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "set up express server", 10),
		asstTurn(base.Add(2*time.Second), "express server set up", 15),
		userTurn(base.Add(5*time.Second), "add a health route to the express server", 10),
		asstTurn(base.Add(8*time.Second), "added health route to express", 12),
	)
	got := topics.Cluster(s, topics.DefaultOptions())
	assert.Len(t, got, 1)
	assert.Equal(t, []int{0, 1, 2, 3}, got[0].TurnIndices)
	assert.Equal(t, 47, got[0].TokenCount)
}

func TestCluster_TimeGap_SplitsAtGap(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "hi", 5),
		asstTurn(base.Add(10*time.Second), "hello", 3),
		userTurn(base.Add(5*time.Minute), "new question", 4),
		asstTurn(base.Add(5*time.Minute+5*time.Second), "answer", 6),
	)
	got := topics.Cluster(s, topics.DefaultOptions())
	assert.Len(t, got, 2)
	assert.Equal(t, []int{0, 1}, got[0].TurnIndices)
	assert.Equal(t, []int{2, 3}, got[1].TurnIndices)
}

func TestCluster_ExplicitMarker_SplitsOnMarker(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "set up express", 10),
		asstTurn(base.Add(2*time.Second), "done", 3),
		userTurn(base.Add(10*time.Minute), "actually, can you write a recipe instead", 10),
		asstTurn(base.Add(10*time.Minute+2*time.Second), "recipe coming up", 5),
	)
	got := topics.Cluster(s, topics.DefaultOptions())
	assert.Len(t, got, 2)
	assert.Equal(t, []int{0, 1}, got[0].TurnIndices)
	assert.Equal(t, []int{2, 3}, got[1].TurnIndices)
}

func TestCluster_TopicFieldsPopulated(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "add jwt auth middleware", 20),
		session.Turn{
			Role:      session.RoleAssistant,
			Timestamp: base.Add(5 * time.Minute),
			Content:   "adding jwt middleware",
			ToolCalls: []session.ToolCall{
				{Name: "Write", Input: []byte(`{"file_path":"/src/auth.go","content":"..."}`)},
				{Name: "Read", Input: []byte(`{"file_path":"/src/auth.go"}`)},
			},
			Tokens: 30,
		},
	)
	got := topics.Cluster(s, topics.DefaultOptions())
	assert.Len(t, got, 2)
	assert.Equal(t, 2, got[1].ToolCallCount)
	assert.ElementsMatch(t, []string{"/src/auth.go"}, got[1].FileSet)
	assert.Equal(t, time.Duration(0), got[1].Duration)
	assert.NotEmpty(t, got[0].Label)
	assert.NotEmpty(t, got[1].Label)
}

func TestCluster_SystemAndSummaryTurns_Excluded(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "work item", 10),
		session.Turn{Role: session.RoleSystem, Timestamp: base.Add(1 * time.Second), Content: "sys"},
		asstTurn(base.Add(2*time.Second), "ok", 5),
	)
	got := topics.Cluster(s, topics.DefaultOptions())
	assert.Len(t, got, 1)
	assert.Equal(t, []int{0, 2}, got[0].TurnIndices)
}

func TestCluster_CompactBoundary_ForcesHardSplit(t *testing.T) {
	// Arrange: two turns very close together in time (would normally be one topic),
	// but a compact boundary sits between them.
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "set up express", 10),
		asstTurn(base.Add(1*time.Second), "express done", 8),
		userTurn(base.Add(2*time.Second), "add rate limiting", 10),
		asstTurn(base.Add(3*time.Second), "rate limiting added", 8),
	)
	// Compact boundary between index 1 and index 2 (TurnIndex=2)
	s.CompactBoundaries = []session.CompactBoundary{
		{TurnIndex: 2, Trigger: "manual", PreTokens: 1000},
	}

	// Act
	got := topics.Cluster(s, topics.DefaultOptions())

	// Assert: must split at boundary regardless of scores
	require.Len(t, got, 2)
	assert.Equal(t, []int{0, 1}, got[0].TurnIndices)
	assert.Equal(t, []int{2, 3}, got[1].TurnIndices)
}

func TestCluster_CompactBoundary_DoesNotSplitWithinSameGroup(t *testing.T) {
	// A boundary at TurnIndex=0 should not create an empty group —
	// if it points to the very first turn it's a no-op for splitting.
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "hello", 5),
		asstTurn(base.Add(1*time.Second), "hi", 3),
	)
	s.CompactBoundaries = []session.CompactBoundary{
		{TurnIndex: 0, Trigger: "manual"},
	}

	got := topics.Cluster(s, topics.DefaultOptions())
	// TurnIndex 0 is the first element in indices; the loop starts at k=1,
	// so the boundary only triggers splits for k > 0. With TurnIndex=0
	// nothing is split by the boundary.
	require.Len(t, got, 1)
}

func TestCluster_MultipleCompactBoundaries_MultipleSplits(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	s := makeSession(
		userTurn(base, "topic a", 5),
		asstTurn(base.Add(1*time.Second), "done a", 3),
		userTurn(base.Add(2*time.Second), "topic b", 5),
		asstTurn(base.Add(3*time.Second), "done b", 3),
		userTurn(base.Add(4*time.Second), "topic c", 5),
		asstTurn(base.Add(5*time.Second), "done c", 3),
	)
	s.CompactBoundaries = []session.CompactBoundary{
		{TurnIndex: 2, Trigger: "manual"},
		{TurnIndex: 4, Trigger: "auto"},
	}

	got := topics.Cluster(s, topics.DefaultOptions())
	require.Len(t, got, 3)
	assert.Equal(t, []int{0, 1}, got[0].TurnIndices)
	assert.Equal(t, []int{2, 3}, got[1].TurnIndices)
	assert.Equal(t, []int{4, 5}, got[2].TurnIndices)
}

func TestCluster_MultiTopicFixture_ThreeBoundaries(t *testing.T) {
	// Arrange
	p := session.NewClaude()
	sess, err := p.Parse(context.Background(), "../../testdata/multi_topic.jsonl")
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Act
	got := topics.Cluster(sess, topics.DefaultOptions())

	// Assert
	assert.Len(t, got, 3, "expected Express / house / Express-health topics")
	for _, top := range got {
		assert.NotEmpty(t, top.Label)
		assert.NotEmpty(t, top.TurnIndices)
	}
}
