package parser

import (
	"context"
	"errors"
)

// Claude parses Claude Code JSONL session files.
//
// TODO(phase-2): streaming JSONL parser per SPEC.md §6.
type Claude struct{}

// NewClaude returns a Claude Code JSONL parser.
func NewClaude() *Claude { return &Claude{} }

// Parse reads the JSONL at path and returns a Session.
func (c *Claude) Parse(_ context.Context, _ string) (*Session, error) {
	return nil, errors.New("not implemented")
}
