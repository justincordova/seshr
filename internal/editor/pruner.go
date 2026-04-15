package editor

import "errors"

// Pruner rewrites a Claude Code JSONL file with selected turns removed.
//
// TODO(phase-5): implement safe pairing, atomic replace, .bak backup.
type Pruner struct{}

// NewPruner returns a Pruner.
func NewPruner() *Pruner { return &Pruner{} }

// Prune removes the turns at the given indices from the JSONL at path.
func (p *Pruner) Prune(_ string, _ []int) error {
	return errors.New("not implemented")
}
