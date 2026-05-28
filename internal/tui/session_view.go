package tui

import (
	"context"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
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
	hadEvictionBefore := v.TurnsLoadedFrom > 0

	v.Session.Turns = append(v.Session.Turns, newTurns...)
	v.TotalTurns += len(newTurns)
	v.TurnsLoadedTo = v.TotalTurns

	if hadEvictionBefore {
		// Re-cluster the in-memory window; topics will reference indices
		// relative to TurnsLoadedFrom. Callers that care about absolute
		// indices must add TurnsLoadedFrom themselves.
		v.Topics = topics.Cluster(v.Session, topics.DefaultOptions())
	} else {
		v.Topics = topics.ClusterAppend(v.Session, topics.DefaultOptions(), v.Topics, newTurns)
	}

	// Evict oldest turns if over the window.
	if len(v.Session.Turns) > maxTurnsInMemory {
		excess := len(v.Session.Turns) - maxTurnsInMemory
		v.Session.Turns = v.Session.Turns[excess:]
		v.TurnsLoadedFrom += excess
	}

	v.Cursor = newCursor
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
