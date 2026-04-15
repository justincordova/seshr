package tokenizer

// Estimate returns an approximate token count for the given text.
//
// TODO(phase-2): prefer JSONL usage fields when present, fall back to
// chars/3.5 heuristic per SPEC.md §7.
func Estimate(_ string) int {
	return 0
}
