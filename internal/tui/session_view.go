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
// Ordering invariant: topics must be clustered against the FINAL
// (post-eviction) window. Clustering first and evicting afterwards would
// leave every topic index shifted left of the slice by the eviction excess —
// and because the shifted index and its timestamp are read from the same
// shifted window, even the editor-side timestamp guard couldn't catch a
// prune computed from them.
//
// The incremental ClusterAppend path is only valid while no eviction has
// EVER occurred (its index math assumes sess.Turns[0] is absolute turn 0);
// any eviction — now or earlier — forces a full O(window) re-cluster, which
// is acceptable given eviction only happens on >500-turn sessions.
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

	// Evict oldest turns if over the window — BEFORE clustering, so topic
	// indices are computed against the slice they'll be used to index.
	evictedNow := false
	if len(v.Session.Turns) > maxTurnsInMemory {
		excess := len(v.Session.Turns) - maxTurnsInMemory
		v.Session.Turns = v.Session.Turns[excess:]
		v.TurnsLoadedFrom += excess
		evictedNow = true
	}

	if hadEvictionBefore || evictedNow {
		// Re-cluster the in-memory window; topics will reference indices
		// relative to TurnsLoadedFrom. Callers that care about absolute
		// indices must add TurnsLoadedFrom themselves.
		v.Topics = topics.Cluster(v.Session, topics.DefaultOptions())
	} else {
		v.Topics = topics.ClusterAppend(v.Session, topics.DefaultOptions(), v.Topics, kept)
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
//
// Resolution is two-phase: targets are resolved first, mutations applied
// only on full success. A partial match is common in a windowed view (the
// issuing turn may have been evicted) and applying it eagerly would leave
// the matched results BOTH folded into the issuing turn AND duplicated in
// the kept standalone turn — rendered twice, tokens counted twice.
//
// Note: unlike the full parser, the fold does NOT record ExtraLineIndices.
// Incrementally parsed turns carry chunk-relative RawIndex values (numbered
// from the cursor's seek position), and ExtraLineIndices is defined as
// absolute file line numbers consumed by the pruner. The editor re-parses
// from disk before pruning, so the view's copy is display-only.
func attachToolResultToView(sess *session.Session, pending []session.Turn, t session.Turn) bool {
	if len(t.ToolResults) == 0 {
		return false
	}

	// Phase 1: resolve a target turn for every result, newest-first, without
	// mutating anything.
	targets := make([]*session.Turn, len(t.ToolResults))
	resolve := func(target *session.Turn) {
		for _, tc := range target.ToolCalls {
			for ri, tr := range t.ToolResults {
				if targets[ri] == nil && tc.ID == tr.ID {
					targets[ri] = target
				}
			}
		}
	}
	for i := len(pending) - 1; i >= 0; i-- {
		resolve(&pending[i])
	}
	for i := len(sess.Turns) - 1; i >= 0; i-- {
		resolve(&sess.Turns[i])
	}
	for _, tgt := range targets {
		if tgt == nil {
			return false
		}
	}

	// Phase 2: every result has a home — apply.
	for ri, tr := range t.ToolResults {
		est := tokenizer.Estimate(tr.Content)
		targets[ri].ToolResults = append(targets[ri].ToolResults, tr)
		targets[ri].Tokens += est
		sess.TokenCount += est
		sess.ToolResultTokens += est
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
