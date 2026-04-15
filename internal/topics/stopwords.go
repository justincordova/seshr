package topics

import "strings"

var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {},
	"but": {}, "by": {}, "can": {}, "do": {}, "does": {}, "for": {}, "from": {},
	"had": {}, "has": {}, "have": {}, "he": {}, "her": {}, "here": {}, "him": {},
	"his": {}, "i": {}, "if": {}, "in": {}, "into": {}, "is": {}, "it": {},
	"its": {}, "just": {}, "let": {}, "like": {}, "me": {}, "my": {}, "no": {},
	"not": {}, "now": {}, "of": {}, "on": {}, "or": {}, "our": {}, "out": {},
	"she": {}, "so": {}, "some": {}, "such": {}, "that": {}, "the": {},
	"their": {}, "them": {}, "then": {}, "there": {}, "these": {}, "they": {},
	"this": {}, "to": {}, "too": {}, "up": {}, "us": {}, "was": {}, "we": {},
	"were": {}, "what": {}, "when": {}, "where": {}, "which": {}, "who": {},
	"will": {}, "with": {}, "would": {}, "you": {}, "your": {}, "yours": {},
	"about": {}, "also": {}, "been": {}, "because": {}, "only": {}, "than": {},
	"very": {}, "all": {}, "any": {}, "each": {}, "more": {}, "most": {},
	"other": {}, "over": {}, "same": {}, "few": {}, "how": {}, "why": {},
}

// IsStopword reports whether w (case-insensitive) is a common English function word.
func IsStopword(w string) bool {
	_, ok := stopwords[strings.ToLower(w)]
	return ok
}
