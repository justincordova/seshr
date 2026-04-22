package topics

import (
	"github.com/justincordova/seshr/internal/session"
)

// ClusterAppend evaluates newTurns against existing clusters without reopening
// historical topics. It is the O(new-turns) incremental counterpart to Cluster.
//
// Invariant: ClusterAppend(sess, opts, Cluster(sess[:n]), sess[n:]) == Cluster(sess).
// This holds as long as no compact boundary straddles the existing/new boundary.
func ClusterAppend(sess *session.Session, opts Options, existing []Topic, newTurns []session.Turn) []Topic {
	if len(existing) == 0 {
		return Cluster(sess, opts)
	}
	if len(newTurns) == 0 {
		return existing
	}

	// Compute the absolute index of the first new turn in sess.Turns.
	// The new turns were appended to sess.Turns immediately before this call.
	firstNewAbs := len(sess.Turns) - len(newTurns)

	result := append([]Topic(nil), existing...) // shallow copy; don't mutate caller's slice
	boundaries := compactBoundarySet(sess)

	for relIdx, tn := range newTurns {
		absIdx := firstNewAbs + relIdx

		if tn.Role == session.RoleSystem || tn.Role == session.RoleSummary {
			continue
		}

		// Hard split at compact boundary.
		if _, isBoundary := boundaries[absIdx]; isBoundary {
			result = append(result, Topic{
				Label:       LabelFor([]session.Turn{tn}, len(result)),
				TurnIndices: []int{absIdx},
				TokenCount:  tn.Tokens,
			})
			continue
		}

		lastTopic := &result[len(result)-1]
		prevAbsIdx := lastTopic.TurnIndices[len(lastTopic.TurnIndices)-1]
		prev := sess.Turns[prevAbsIdx]

		score := TimeGapScore(prev, tn, opts) +
			ExplicitMarkerScore(prev, tn) +
			FileShiftScore(ExtractFiles(prev.ToolCalls), ExtractFiles(tn.ToolCalls), opts) +
			KeywordScore(prev, tn, opts)

		if score >= opts.BoundaryThreshold {
			result = append(result, Topic{
				Label:       LabelFor([]session.Turn{tn}, len(result)),
				TurnIndices: []int{absIdx},
				TokenCount:  tn.Tokens,
			})
		} else {
			// Extend the last topic.
			lastTopic.TurnIndices = append(lastTopic.TurnIndices, absIdx)
			lastTopic.TokenCount += tn.Tokens
			lastTopic.ToolCallCount += len(tn.ToolCalls)
			for _, f := range ExtractFiles(tn.ToolCalls) {
				addToFileSet(lastTopic, f)
			}
			// Duration: update to span from first to current.
			first := sess.Turns[lastTopic.TurnIndices[0]]
			if !first.Timestamp.IsZero() && !tn.Timestamp.IsZero() {
				lastTopic.Duration = tn.Timestamp.Sub(first.Timestamp)
			}
		}
	}

	return result
}

// addToFileSet adds a file path to a topic's FileSet without duplicates.
func addToFileSet(t *Topic, path string) {
	for _, f := range t.FileSet {
		if f == path {
			return
		}
	}
	t.FileSet = append(t.FileSet, path)
}
