package topics

import (
	"sort"
	"time"

	"github.com/justincordova/seshr/internal/session"
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

// compactBoundarySet returns a set of turn indices that begin a new compact
// segment. A compact boundary at TurnIndex n means: if indices[k] == n (or the
// previous index was < n and current is >= n), force a split.
func compactBoundarySet(sess *session.Session) map[int]struct{} {
	m := make(map[int]struct{}, len(sess.CompactBoundaries))
	for _, cb := range sess.CompactBoundaries {
		m[cb.TurnIndex] = struct{}{}
	}
	return m
}

// Cluster groups sess.Turns into topics. System and summary turns are excluded.
// Compact boundaries (from /compact calls) are treated as hard splits —
// no topic may span a compact boundary regardless of other signal scores.
func Cluster(sess *session.Session, opts Options) []Topic {
	if sess == nil || len(sess.Turns) == 0 {
		return nil
	}
	var indices []int
	for i, t := range sess.Turns {
		if t.Role == session.RoleSystem || t.Role == session.RoleSummary {
			continue
		}
		indices = append(indices, i)
	}
	if len(indices) == 0 {
		return nil
	}

	boundaries := compactBoundarySet(sess)

	groups := [][]int{{indices[0]}}
	for k := 1; k < len(indices); k++ {
		prev := sess.Turns[indices[k-1]]
		cur := sess.Turns[indices[k]]

		// Hard split: compact boundary falls at or before this turn index
		// and after the previous turn index.
		if _, isBoundary := boundaries[indices[k]]; isBoundary {
			groups = append(groups, []int{indices[k]})
			continue
		}

		score := TimeGapScore(prev, cur, opts) +
			ExplicitMarkerScore(prev, cur) +
			FileShiftScore(ExtractFiles(prev.ToolCalls), ExtractFiles(cur.ToolCalls), opts) +
			KeywordScore(prev, cur, opts)
		if score >= opts.BoundaryThreshold {
			groups = append(groups, []int{indices[k]})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], indices[k])
		}
	}

	out := make([]Topic, 0, len(groups))
	for i, g := range groups {
		out = append(out, buildTopic(sess, g, i))
	}
	return out
}

func buildTopic(sess *session.Session, group []int, idx int) Topic {
	turns := make([]session.Turn, 0, len(group))
	var tokens, tools int
	fileSet := map[string]struct{}{}
	var first, last session.Turn
	for i, ti := range group {
		tn := sess.Turns[ti]
		turns = append(turns, tn)
		tokens += tn.Tokens
		// ToolCalls holds tool_use invocations only; tool results are in Turn.ToolResults.
		tools += len(tn.ToolCalls)
		for _, f := range ExtractFiles(tn.ToolCalls) {
			fileSet[f] = struct{}{}
		}
		if i == 0 {
			first = tn
		}
		last = tn
	}
	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}
	sort.Strings(files)
	var dur time.Duration
	if !first.Timestamp.IsZero() && !last.Timestamp.IsZero() {
		dur = last.Timestamp.Sub(first.Timestamp)
	}
	return Topic{
		Label:         LabelFor(turns, idx),
		TurnIndices:   append([]int(nil), group...),
		TokenCount:    tokens,
		ToolCallCount: tools,
		Duration:      dur,
		FileSet:       files,
	}
}
