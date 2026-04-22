package topics

import (
	"encoding/json"

	"github.com/justincordova/seshr/internal/session"
)

// ExtractFiles pulls unique file paths from tool-call Input JSON blobs.
// Only file-oriented tools are inspected; Bash is intentionally skipped
// (heuristic false-positive rate too high).
func ExtractFiles(calls []session.ToolCall) []string {
	seen := make(map[string]struct{})
	for _, c := range calls {
		if c.Name == "Bash" {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(c.Input, &m); err != nil {
			continue
		}
		for _, key := range []string{"file_path", "path", "notebook_path"} {
			if raw, ok := m[key]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" {
					seen[s] = struct{}{}
				}
			}
		}
		// Glob tool uses "pattern" instead of a file path key.
		if c.Name == "Glob" {
			if raw, ok := m["pattern"]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" {
					seen[s] = struct{}{}
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}

// Jaccard returns the Jaccard similarity coefficient between two string sets.
// Both nil/empty → returns 1.0 (identical empty sets). One empty → 0.0.
func Jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}
	var intersection int
	setB := make(map[string]struct{}, len(b))
	for _, s := range b {
		if _, ok := setA[s]; ok {
			intersection++
		}
		setB[s] = struct{}{}
	}
	union := len(setA) + len(setB) - intersection
	return float64(intersection) / float64(union)
}
