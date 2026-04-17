package topics

import (
	"fmt"
	"strings"

	"github.com/justincordova/agentlens/internal/parser"
)

const labelMaxLen = 40

// LabelFor returns a label for a topic at 0-based position idx.
// Fallback order: top-3 keywords → first user message → "Topic N".
func LabelFor(turns []parser.Turn, idx int) string {
	if len(turns) == 0 {
		return fmt.Sprintf("Topic %d", idx+1)
	}
	var combined strings.Builder
	for _, tn := range turns {
		combined.WriteString(tn.Content)
		combined.WriteByte(' ')
	}
	if kws := topKeywords(combined.String(), 3); len(kws) > 0 {
		return truncateLabel(strings.Join(kws, " "))
	}
	for _, tn := range turns {
		if tn.Role == parser.RoleUser && strings.TrimSpace(tn.Content) != "" {
			return truncateLabel(strings.TrimSpace(tn.Content))
		}
	}
	return fmt.Sprintf("Topic %d", idx+1)
}

func truncateLabel(s string) string {
	runes := []rune(s)
	if len(runes) <= labelMaxLen {
		return s
	}
	return string(runes[:labelMaxLen-1]) + "…"
}
