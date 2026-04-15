package topics_test

import (
	"testing"
	"time"

	"github.com/justincordova/agentlens/internal/topics"
	"github.com/stretchr/testify/assert"
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
