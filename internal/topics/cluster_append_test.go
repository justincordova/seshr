package topics_test

import (
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterAppend_EqualsFullCluster(t *testing.T) {
	// Invariant: ClusterAppend with first-half existing + second-half new equals Cluster on full.
	base := time.Unix(1_700_000_000, 0)
	allTurns := []session.Turn{
		{Role: session.RoleUser, Timestamp: base, Content: "set up express", Tokens: 10},
		{Role: session.RoleAssistant, Timestamp: base.Add(2 * time.Second), Content: "done", Tokens: 5},
		{Role: session.RoleUser, Timestamp: base.Add(10 * time.Minute), Content: "new topic", Tokens: 10},
		{Role: session.RoleAssistant, Timestamp: base.Add(10*time.Minute + 2*time.Second), Content: "ok", Tokens: 5},
	}

	sess := &session.Session{Turns: allTurns}
	opts := topics.DefaultOptions()

	// Full cluster.
	fullResult := topics.Cluster(sess, opts)

	// Now simulate: first two turns were the existing session.
	sessHalf := &session.Session{Turns: allTurns[:2]}
	existing := topics.Cluster(sessHalf, topics.DefaultOptions())

	// Append the remaining turns to the full session.
	fullSess := &session.Session{Turns: allTurns}
	incremental := topics.ClusterAppend(fullSess, opts, existing, allTurns[2:])

	// Assert: same number of topics.
	require.Equal(t, len(fullResult), len(incremental), "topic counts must match")
	for i := range fullResult {
		assert.Equal(t, fullResult[i].TurnIndices, incremental[i].TurnIndices,
			"topic %d turn indices must match", i)
	}
}

func TestClusterAppend_EmptyExisting_DelegatesToCluster(t *testing.T) {
	// Arrange
	base := time.Unix(1_700_000_000, 0)
	sess := &session.Session{Turns: []session.Turn{
		{Role: session.RoleUser, Timestamp: base, Content: "hello", Tokens: 5},
	}}

	// Act
	result := topics.ClusterAppend(sess, topics.DefaultOptions(), nil, sess.Turns)

	// Assert
	assert.Len(t, result, 1)
}

func TestClusterAppend_CompactBoundary_ForcesNewTopic(t *testing.T) {
	// Arrange
	base := time.Unix(1_700_000_000, 0)
	allTurns := []session.Turn{
		{Role: session.RoleUser, Timestamp: base, Content: "a", Tokens: 5},
		{Role: session.RoleAssistant, Timestamp: base.Add(1 * time.Second), Content: "b", Tokens: 5},
		{Role: session.RoleUser, Timestamp: base.Add(2 * time.Second), Content: "c", Tokens: 5},
	}

	sess := &session.Session{
		Turns:             allTurns,
		CompactBoundaries: []session.CompactBoundary{{TurnIndex: 2}},
	}

	existing := topics.Cluster(&session.Session{Turns: allTurns[:2]}, topics.DefaultOptions())
	result := topics.ClusterAppend(sess, topics.DefaultOptions(), existing, allTurns[2:])

	// Assert: the compact boundary forces a new topic at index 2.
	require.Len(t, result, 2)
	assert.Equal(t, []int{2}, result[1].TurnIndices)
}
