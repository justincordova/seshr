package parser

import "context"

// SessionParser parses a JSONL session file on disk into a Session.
//
// TODO(phase-2): implement Claude Code parser.
type SessionParser interface {
	Parse(ctx context.Context, path string) (*Session, error)
}
