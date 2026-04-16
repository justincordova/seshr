package topics

import (
	"strings"

	"github.com/justincordova/agentlens/internal/parser"
)

const (
	weightTimeGap        = 0.45
	weightExplicitMarker = 0.40
	weightFileShift      = 0.25
	weightKeyword        = 0.15
)

var explicitMarkers = []string{
	"let's move on", "lets move on", "move on", "new topic",
	"switching to", "actually, can you", "actually can you",
	"unrelated but", "unrelated:", "switching gears", "switch gears",
	"change of topic", "different question", "next topic",
}

// TimeGapScore returns weightTimeGap when the gap strictly exceeds opts.GapThreshold.
func TimeGapScore(prev, cur parser.Turn, opts Options) float64 {
	if prev.Timestamp.IsZero() || cur.Timestamp.IsZero() {
		return 0
	}
	if cur.Timestamp.Sub(prev.Timestamp) > opts.GapThreshold {
		return weightTimeGap
	}
	return 0
}

// ExplicitMarkerScore returns weightExplicitMarker when cur is a user turn
// containing one of the explicit marker phrases.
func ExplicitMarkerScore(_, cur parser.Turn) float64 {
	if cur.Role != parser.RoleUser {
		return 0
	}
	low := strings.ToLower(cur.Content)
	for _, m := range explicitMarkers {
		if strings.Contains(low, m) {
			return weightExplicitMarker
		}
	}
	return 0
}

// FileShiftScore returns weightFileShift when Jaccard(prev, cur) < opts.FileJaccardThreshold.
// Two empty sets don't signal a shift.
func FileShiftScore(prev, cur []string, opts Options) float64 {
	if len(prev) == 0 && len(cur) == 0 {
		return 0
	}
	if Jaccard(prev, cur) < opts.FileJaccardThreshold {
		return weightFileShift
	}
	return 0
}

// KeywordScore returns weightKeyword when keyword overlap < opts.KeywordOverlapThreshold.
func KeywordScore(prev, cur parser.Turn, opts Options) float64 {
	kPrev := topKeywords(prev.Content, 5)
	kCur := topKeywords(cur.Content, 5)
	if len(kPrev) == 0 || len(kCur) == 0 {
		return 0
	}
	if keywordOverlap(kPrev, kCur) < opts.KeywordOverlapThreshold {
		return weightKeyword
	}
	return 0
}

func topKeywords(text string, n int) []string {
	freq := map[string]int{}
	for _, tok := range tokenize(text) {
		low := strings.ToLower(tok)
		if len(low) < 3 || IsStopword(low) {
			continue
		}
		freq[low]++
	}
	type kv struct {
		word string
		n    int
	}
	var items []kv
	for w, c := range freq {
		items = append(items, kv{w, c})
	}
	for i := 0; i < len(items) && i < n; i++ {
		best := i
		for j := i + 1; j < len(items); j++ {
			if items[j].n > items[best].n ||
				(items[j].n == items[best].n && items[j].word < items[best].word) {
				best = j
			}
		}
		items[i], items[best] = items[best], items[i]
	}
	if len(items) > n {
		items = items[:n]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.word)
	}
	return out
}

func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '_'
	})
}

func keywordOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := map[string]struct{}{}
	for _, w := range a {
		set[w] = struct{}{}
	}
	var inter int
	for _, w := range b {
		if _, ok := set[w]; ok {
			inter++
		}
	}
	min := len(a)
	if len(b) < min {
		min = len(b)
	}
	return float64(inter) / float64(min)
}
