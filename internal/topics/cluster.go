package topics

import (
	"time"

	"github.com/justincordova/agentlens/internal/parser"
)

// Topic is a contiguous run of turns grouped by a shared subject.
// See SPEC §3.2 / §5.
type Topic struct {
	Label         string
	TurnIndices   []int         // indices into the owning Session.Turns
	TokenCount    int           // sum of tokens across the topic's turns
	ToolCallCount int           // tool_use invocations (not results)
	Duration      time.Duration // last.Timestamp - first.Timestamp
	FileSet       []string      // unique file paths referenced by tool calls
}

// Options carries the tunable thresholds for the clustering algorithm.
type Options struct {
	GapThreshold            time.Duration
	FileJaccardThreshold    float64 // below this = file-context shift signal
	KeywordOverlapThreshold float64 // below this = keyword-divergence signal
	BoundaryThreshold       float64 // summed signal score that triggers a boundary
}

// DefaultOptions returns the v1 defaults from SPEC §5.
func DefaultOptions() Options {
	return Options{
		GapThreshold:            3 * time.Minute,
		FileJaccardThreshold:    0.3,
		KeywordOverlapThreshold: 0.2,
		BoundaryThreshold:       0.5,
	}
}

// Cluster groups a session's turns into topics using the signals from SPEC §5.
// TODO(phase-3): full implementation in later tasks.
func Cluster(_ *parser.Session, _ Options) []Topic {
	return nil
}
