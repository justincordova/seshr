package topics_test

import (
	"testing"
	"time"

	"github.com/justincordova/agentlens/internal/topics"
	"github.com/stretchr/testify/assert"
)

func TestCluster_NilSession_ReturnsNil(t *testing.T) {
	// Arrange / Act
	got := topics.Cluster(nil, 3*time.Minute)

	// Assert
	assert.Nil(t, got)
}
