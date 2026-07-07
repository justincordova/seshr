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
func (v *SessionView) Append(newTurns []session.Turn, boundaries []session.CompactBoundary, newCursor backend.Cursor) {
	if len(newTurns) == 0 {
		// A tick may carry only a compact boundary (the /compact record with
		// no renderable turn yet). Still merge it so clustering and prune
		// safety see the live compaction.
		v.mergeBoundaries(boundaries, len(v.Session.Turns), nil)
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
	// keptBefore[si] = number of kept turns among newTurns[:si]. Boundary
	// TurnIndex values are stream-relative to the UNFOLDED newTurns; the fold
	// drops tool-result turns, so a boundary's kept-index is keptBefore[si].
	keptBefore := make([]int, len(newTurns)+1)
	for i, t := range newTurns {
		keptBefore[i] = len(kept)
		if t.Role == session.RoleToolResult && attachToolResultToView(v.Session, kept, t) {
			continue
		}
		kept = append(kept, t)
	}
	keptBefore[len(newTurns)] = len(kept)

	base := len(v.Session.Turns)

	if len(kept) == 0 {
		v.mergeBoundaries(boundaries, base, keptBefore)
		v.Cursor = newCursor
		return
	}

	hadEvictionBefore := v.TurnsLoadedFrom > 0

	// Merge boundaries BEFORE appending/clustering: base is the absolute index
	// of the first kept turn, and mergeBoundaries maps each stream-relative
	// boundary onto its post-fold absolute index. Clustering below then sees
	// them and can force the compact hard split.
	v.mergeBoundaries(boundaries, base, keptBefore)

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
		// Re-base compact boundaries onto the shifted window and drop any that
		// fell out of it. Cluster/ClusterAppend read CompactBoundaries as
		// physical indices into v.Session.Turns; leaving them at their pre-
		// eviction positions would force topic hard-splits at the wrong turns.
		v.rebaseBoundaries(excess)
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

// mergeBoundaries appends incrementally-parsed compact boundaries to the
// session, translating each from its stream-relative TurnIndex to an absolute
// index into v.Session.Turns.
//
// base is the absolute index of the first turn in this delta. keptBefore, when
// non-nil, maps a stream (unfolded) turn index to the count of kept turns
// before it, so a boundary that sat before unfolded turn si lands at
// base+keptBefore[si] — the same absolute slot the surviving turn occupies
// after tool-result folding. When keptBefore is nil (no renderable turns in the
// delta) every boundary collapses onto base.
func (v *SessionView) mergeBoundaries(boundaries []session.CompactBoundary, base int, keptBefore []int) {
	if len(boundaries) == 0 {
		return
	}
	existing := make(map[int]struct{}, len(v.Session.CompactBoundaries))
	for _, cb := range v.Session.CompactBoundaries {
		existing[cb.TurnIndex] = struct{}{}
	}
	for _, cb := range boundaries {
		abs := base
		if keptBefore != nil {
			si := cb.TurnIndex
			if si < 0 {
				si = 0
			}
			if si >= len(keptBefore) {
				si = len(keptBefore) - 1
			}
			abs = base + keptBefore[si]
		}
		if _, dup := existing[abs]; dup {
			continue
		}
		cb.TurnIndex = abs
		v.Session.CompactBoundaries = append(v.Session.CompactBoundaries, cb)
		existing[abs] = struct{}{}
	}
}

// rebaseBoundaries shifts every compact boundary left by excess (the number of
// turns just evicted from the front of the window) and drops any that now fall
// before the window. Keeps CompactBoundaries aligned with the re-based
// v.Session.Turns slice that Cluster indexes.
func (v *SessionView) rebaseBoundaries(excess int) {
	if excess <= 0 || len(v.Session.CompactBoundaries) == 0 {
		return
	}
	kept := v.Session.CompactBoundaries[:0]
	for _, cb := range v.Session.CompactBoundaries {
		cb.TurnIndex -= excess
		if cb.TurnIndex < 0 {
			continue
		}
		kept = append(kept, cb)
	}
	v.Session.CompactBoundaries = kept
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
