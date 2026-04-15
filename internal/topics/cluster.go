package topics

import (
	"time"

	"github.com/justincordova/agentlens/internal/parser"
)

// Topic is a contiguous run of turns grouped by a shared subject.
//
// TODO(phase-3): fill from clustering per SPEC.md §5.
type Topic struct {
	Label         string
	TurnIndices   []int
	TokenCount    int
	ToolCallCount int
	Duration      time.Duration
}

// Cluster groups a session's turns into topics.
//
// TODO(phase-3): implement time-gap / file-shift / explicit-marker detection.
func Cluster(_ *parser.Session, _ time.Duration) []Topic {
	return nil
}
