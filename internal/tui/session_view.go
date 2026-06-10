package tui

import (
	"context"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tokenizer"
	"github.com/justincordova/seshr/internal/topics"
)

const maxTurnsInMemory = 500

// SessionView holds per-session live state shared across Landing, Topics, and
// Replay. One source of truth per open session.
type SessionView struct {
	Meta    backend.SessionMeta
	Session *session.Session
	Topics  []topics.Topic
	Cursor  backend.Cursor
	Live    *backend.LiveSession // nil when ended

	TurnsLoadedFrom int
	TurnsLoadedTo   int
	TotalTurns      int

	store backend.SessionStore
}

// NewSessionView loads the session via the store and clusters it.
func NewSessionView(ctx context.Context, store backend.SessionStore, meta backend.SessionMeta) (*SessionView, error) {
	sess, cur, err := store.Load(ctx, meta.ID)
	if err != nil {
		return nil, err
	}
	tpcs := topics.Cluster(sess, topics.DefaultOptions())
	n := len(sess.Turns)
	return &SessionView{
		Meta:            meta,
		Session:         sess,
		Topics:          tpcs,
		Cursor:          cur,
		TurnsLoadedFrom: 0,
		TurnsLoadedTo:   n,
		TotalTurns:      n,
		store:           store,
	}, nil
}

// Append adds new turns to the view, evicting oldest if the window is full.
// Topics are updated via ClusterAppend.
//
// Clustering happens BEFORE eviction so ClusterAppend's `len(sess.Turns) -
// len(newTurns)` base equals the absolute index of the first new turn in
// the current window. Eviction would shrink sess.Turns out from under the
// existing topics' absolute indices and the next ClusterAppend would index
// past the slice end.
//
// NOTE: existing topic indices already carry absolute (pre-eviction) values.
// Once eviction has happened in a previous Append, ClusterAppend's index
// math against sess.Turns[prevAbsIdx] would be wrong, so we defensively
// skip incremental clustering and rebuild from scratch in that case. The
// cost is O(turns-in-window) re-cluster instead of O(newTurns); acceptable
// given eviction only happens on >500-turn sessions and not on every tick.
func (v *SessionView) Append(newTurns []session.Turn, newCursor backend.Cursor) {
	if len(newTurns) == 0 {
		v.Cursor = newCursor
		return
	}

	// Fold tool-result turns into the assistant turn that issued the call,
	// mirroring the claude full parser's attachToolResult. The incremental
	// stream parser cannot do this itself: the issuing assistant turn was
	// usually returned by an EARLIER tick and exists only in this view.
	// Without the fold, the live view's turn-index space drifts one right of
	// a fresh Parse per attached result — wrong turn counts, and prune
	// selections that the editor-side staleness guard then has to refuse.
	kept := make([]session.Turn, 0, len(newTurns))
	for _, t := range newTurns {
		if t.Role == session.RoleToolResult && attachToolResultToView(v.Session, kept, t) {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		v.Cursor = newCursor
		return
	}

	hadEvictionBefore := v.TurnsLoadedFrom > 0

	v.Session.Turns = append(v.Session.Turns, kept...)
	v.TotalTurns += len(kept)
	v.TurnsLoadedTo = v.TotalTurns

	if hadEvictionBefore {
		// Re-cluster the in-memory window; topics will reference indices
		// relative to TurnsLoadedFrom. Callers that care about absolute
		// indices must add TurnsLoadedFrom themselves.
		v.Topics = topics.Cluster(v.Session, topics.DefaultOptions())
	} else {
		v.Topics = topics.ClusterAppend(v.Session, topics.DefaultOptions(), v.Topics, kept)
	}

	// Evict oldest turns if over the window.
	if len(v.Session.Turns) > maxTurnsInMemory {
		excess := len(v.Session.Turns) - maxTurnsInMemory
		v.Session.Turns = v.Session.Turns[excess:]
		v.TurnsLoadedFrom += excess
	}

	v.Cursor = newCursor
}

// attachToolResultToView merges a tool-result turn's results into the
// assistant turn that issued the matching tool calls, searching the current
// batch (pending) first, then the view's existing turns, newest first.
// Returns true only when EVERY result found a home — the caller drops the
// standalone turn in that case, matching the claude parser's semantics
// (jsonl.go attachToolResult). Token accounting mirrors the parser too, so
// the view's aggregates stay comparable to a fresh Load.
func attachToolResultToView(sess *session.Session, pending []session.Turn, t session.Turn) bool {
	if len(t.ToolResults) == 0 {
		return false
	}
	attached := make([]bool, len(t.ToolResults))
	tryAttach := func(target *session.Turn) {
		for _, tc := range target.ToolCalls {
			for ri, tr := range t.ToolResults {
				if attached[ri] || tc.ID != tr.ID {
					continue
				}
				est := tokenizer.Estimate(tr.Content)
				target.ToolResults = append(target.ToolResults, tr)
				target.Tokens += est
				target.ExtraLineIndices = append(target.ExtraLineIndices, t.RawIndex)
				sess.TokenCount += est
				sess.ToolResultTokens += est
				attached[ri] = true
			}
		}
	}
	for i := len(pending) - 1; i >= 0; i-- {
		tryAttach(&pending[i])
	}
	for i := len(sess.Turns) - 1; i >= 0; i-- {
		tryAttach(&sess.Turns[i])
	}
	for _, ok := range attached {
		if !ok {
			return false
		}
	}
	return true
}

// Reset replaces the view's contents with a freshly loaded session. Used
// when the store reports backend.ErrCursorInvalid (file rotated, truncated,
// or replaced by a prune) — appending in that state would duplicate turns.
func (v *SessionView) Reset(sess *session.Session, cur backend.Cursor) {
	v.Session = sess
	v.Topics = topics.Cluster(sess, topics.DefaultOptions())
	v.Cursor = cur
	v.TurnsLoadedFrom = 0
	v.TurnsLoadedTo = len(sess.Turns)
	v.TotalTurns = len(sess.Turns)
}

// LoadRange replaces the in-memory window with an arbitrary slice.
// Topics are NOT recomputed — they hold logical indices into the full session.
func (v *SessionView) LoadRange(ctx context.Context, from, to int) error {
	turns, err := v.store.LoadRange(ctx, v.Meta.ID, from, to)
	if err != nil {
		return err
	}
	v.Session.Turns = turns
	v.TurnsLoadedFrom = from
	v.TurnsLoadedTo = to
	return nil
}

// HasTurn reports whether logical turn index idx is currently in memory.
func (v *SessionView) HasTurn(idx int) bool {
	return idx >= v.TurnsLoadedFrom && idx < v.TurnsLoadedTo
}
