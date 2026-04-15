package tokenizer

import "math"

// Usage mirrors the usage subfield that Claude Code includes on assistant
// JSONL records. Cache fields are summed as billable context per SPEC §7.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// Estimate returns an approximate token count for the given text using the
// len/3.5 heuristic from SPEC §7. Within ~10-15% of real Claude tokenization
// for English; always prefer FromUsage when real counts are available.
func Estimate(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Round(float64(len(text)) / 3.5))
}

// FromUsage returns the total token count from a usage field, summing input,
// cache-creation, cache-read, and output tokens.
func FromUsage(u Usage) int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}
