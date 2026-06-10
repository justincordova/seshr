package backend

import (
	"errors"
	"time"
)

// ErrSelectionStale is returned by SessionEditor.Prune when the session's
// turn list no longer matches the one the selection was computed against
// (e.g. the live agent rewrote/compacted the transcript, or the view's
// index space drifted). The caller should reload the session and re-select.
var ErrSelectionStale = errors.New("session changed since selection was made; reload and retry")

// Selection carries the set of turn indices to prune, as accepted by SessionEditor.
// TurnIndices are indices into session.Session.Turns.
//
// TurnTimestamps, when non-empty, is parallel to TurnIndices and carries the
// Timestamp of each selected turn as the UI saw it at selection time. Editors
// MUST verify each timestamp against their own fresh view of the session
// before deleting anything and return ErrSelectionStale on mismatch — turn
// indices are positional, and a transcript rewrite between selection and
// prune would otherwise silently delete the wrong turns.
type Selection struct {
	TurnIndices    []int
	TurnTimestamps []time.Time
}
