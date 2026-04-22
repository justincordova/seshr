package tui

import (
	"sync"
	"time"

	"github.com/justincordova/seshr/internal/backend"
)

// liveEntry is an entry in the live index.
type liveEntry struct {
	live     *backend.LiveSession
	lastSeen time.Time
}

// Transition records a change in a session's live state.
type Transition struct {
	SessionID string
	Prev      *backend.LiveSession // nil → newly live
	Next      *backend.LiveSession // nil → ended
}

// LiveIndex is a concurrency-safe index of known live sessions with hysteresis.
//
// Upgrade (ended → live, or Waiting → Working) is instant.
// Downgrade (Working → Waiting, or live → ended) requires 2 missed ticks.
type LiveIndex struct {
	mu      sync.RWMutex
	items   map[string]*liveEntry
	missed  map[string]int // ticks missed since last sighting
	pending map[string]int // downgrade-pending count (Working → Waiting)
}

// NewLiveIndex returns an empty LiveIndex.
func NewLiveIndex() *LiveIndex {
	return &LiveIndex{
		items:   make(map[string]*liveEntry),
		missed:  make(map[string]int),
		pending: make(map[string]int),
	}
}

// Lookup returns the live session for id, or nil if not live.
func (l *LiveIndex) Lookup(id string) *backend.LiveSession {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if e, ok := l.items[id]; ok {
		return e.live
	}
	return nil
}

// Snapshot returns a slice of all currently live sessions.
func (l *LiveIndex) Snapshot() []*backend.LiveSession {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*backend.LiveSession, 0, len(l.items))
	for _, e := range l.items {
		out = append(out, e.live)
	}
	return out
}

// Update replaces the entry for id (used by fast tick to update CurrentTask).
func (l *LiveIndex) Update(id string, next *backend.LiveSession) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.items[id]; ok {
		e.live = next
		e.lastSeen = time.Now()
	}
}

// Reconcile replaces the index with detector results, applying hysteresis.
// It returns the set of transitions that occurred.
func (l *LiveIndex) Reconcile(detected []*backend.LiveSession) []Transition {
	l.mu.Lock()
	defer l.mu.Unlock()

	incoming := make(map[string]*backend.LiveSession, len(detected))
	for _, s := range detected {
		incoming[s.SessionID] = s
	}

	var transitions []Transition

	// Process existing entries.
	for id, entry := range l.items {
		next, inIncoming := incoming[id]
		if !inIncoming {
			// Disappeared this tick.
			l.missed[id]++
			if l.missed[id] >= 2 {
				// Confirmed ended.
				transitions = append(transitions, Transition{SessionID: id, Prev: entry.live, Next: nil})
				delete(l.items, id)
				delete(l.missed, id)
				delete(l.pending, id)
			}
			// else: keep for another tick
			continue
		}
		// In incoming — reset missed counter.
		l.missed[id] = 0

		prev := entry.live

		// Upgrade: Waiting → Working is instant.
		if prev.Status == backend.StatusWaiting && next.Status == backend.StatusWorking {
			entry.live = next
			entry.lastSeen = time.Now()
			delete(l.pending, id)
			transitions = append(transitions, Transition{SessionID: id, Prev: prev, Next: next})
			continue
		}

		// Downgrade: Working → Waiting requires 2 ticks.
		if prev.Status == backend.StatusWorking && next.Status == backend.StatusWaiting {
			l.pending[id]++
			if l.pending[id] >= 2 {
				entry.live = next
				entry.lastSeen = time.Now()
				delete(l.pending, id)
				transitions = append(transitions, Transition{SessionID: id, Prev: prev, Next: next})
			}
			// else: keep Working for one more tick; no transition emitted
			continue
		}

		// Status equal or other combination — always update fields, emit transition.
		delete(l.pending, id)
		entry.live = next
		entry.lastSeen = time.Now()
		transitions = append(transitions, Transition{SessionID: id, Prev: prev, Next: next})
	}

	// New entries not in the old index.
	for id, next := range incoming {
		if _, exists := l.items[id]; !exists {
			l.items[id] = &liveEntry{live: next, lastSeen: time.Now()}
			transitions = append(transitions, Transition{SessionID: id, Prev: nil, Next: next})
		}
	}

	return transitions
}
