package tui_test

import (
	"testing"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveSession(id string, status backend.Status) *backend.LiveSession {
	return &backend.LiveSession{
		SessionID:    id,
		Kind:         session.SourceClaude,
		Status:       status,
		LastActivity: time.Now(),
	}
}

func TestLiveIndex_NewEntry_EmitsTransition(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	s := liveSession("abc", backend.StatusWorking)

	// Act
	transitions := idx.Reconcile([]*backend.LiveSession{s})

	// Assert
	require.Len(t, transitions, 1)
	assert.Nil(t, transitions[0].Prev)
	assert.Equal(t, "abc", transitions[0].Next.SessionID)
}

func TestLiveIndex_SameWorking_UpdatesEntry(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	s := liveSession("abc", backend.StatusWorking)
	idx.Reconcile([]*backend.LiveSession{s})

	// Act: same session, same status.
	transitions := idx.Reconcile([]*backend.LiveSession{s})

	// Assert: transition emitted with equal prev/next (field update path).
	assert.Len(t, transitions, 1)
	assert.NotNil(t, transitions[0].Prev)
	assert.NotNil(t, transitions[0].Next)
}

func TestLiveIndex_Working_MissingOnce_StillLive(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	idx.Reconcile([]*backend.LiveSession{liveSession("abc", backend.StatusWorking)})

	// Act: missing for one tick.
	transitions := idx.Reconcile(nil)

	// Assert: still live, no transition yet.
	assert.Empty(t, transitions)
	assert.NotNil(t, idx.Lookup("abc"))
}

func TestLiveIndex_Working_MissingTwice_Ended(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	idx.Reconcile([]*backend.LiveSession{liveSession("abc", backend.StatusWorking)})
	idx.Reconcile(nil) // missed once

	// Act: missed twice.
	transitions := idx.Reconcile(nil)

	// Assert: ended.
	require.Len(t, transitions, 1)
	assert.NotNil(t, transitions[0].Prev)
	assert.Nil(t, transitions[0].Next)
	assert.Nil(t, idx.Lookup("abc"))
}

func TestLiveIndex_WorkingToWaiting_RequiresTwoTicks(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	working := liveSession("abc", backend.StatusWorking)
	idx.Reconcile([]*backend.LiveSession{working})

	// Act: downgrade in one tick.
	waiting := liveSession("abc", backend.StatusWaiting)
	transitions := idx.Reconcile([]*backend.LiveSession{waiting})

	// Assert: no immediate downgrade — still Working.
	assert.Empty(t, transitions)
	live := idx.Lookup("abc")
	require.NotNil(t, live)
	assert.Equal(t, backend.StatusWorking, live.Status)

	// Second downgrade tick.
	transitions2 := idx.Reconcile([]*backend.LiveSession{waiting})
	assert.Len(t, transitions2, 1)
	live2 := idx.Lookup("abc")
	require.NotNil(t, live2)
	assert.Equal(t, backend.StatusWaiting, live2.Status)
}

func TestLiveIndex_WaitingToWorking_Instant(t *testing.T) {
	// Arrange
	idx := tui.NewLiveIndex()
	idx.Reconcile([]*backend.LiveSession{liveSession("abc", backend.StatusWaiting)})

	// Act: upgrade.
	transitions := idx.Reconcile([]*backend.LiveSession{liveSession("abc", backend.StatusWorking)})

	// Assert: immediate upgrade.
	require.Len(t, transitions, 1)
	assert.Equal(t, backend.StatusWaiting, transitions[0].Prev.Status)
	assert.Equal(t, backend.StatusWorking, transitions[0].Next.Status)
}

func TestShouldRunFastTick(t *testing.T) {
	assert.True(t, tui.ShouldRunFastTick(3, false))
	assert.False(t, tui.ShouldRunFastTick(0, false))
	assert.False(t, tui.ShouldRunFastTick(3, true))
}
