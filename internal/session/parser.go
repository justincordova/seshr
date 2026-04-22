package session

import "context"

// SessionParser parses a JSONL session file on disk into a Session.
type SessionParser interface {
	Parse(ctx context.Context, path string) (*Session, error)
}
