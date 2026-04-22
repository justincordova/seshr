package backend_test

import (
	"context"
	"errors"
	"testing"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStore satisfies backend.SessionStore for testing.
type stubStore struct {
	kind     session.SourceKind
	closeErr error
}

func (s *stubStore) Kind() session.SourceKind                              { return s.kind }
func (s *stubStore) Scan(_ context.Context) ([]backend.SessionMeta, error) { return nil, nil }
func (s *stubStore) Load(_ context.Context, _ string) (*session.Session, backend.Cursor, error) {
	return nil, backend.Cursor{}, nil
}
func (s *stubStore) LoadIncremental(_ context.Context, _ string, cur backend.Cursor) ([]session.Turn, backend.Cursor, error) {
	return nil, cur, nil
}
func (s *stubStore) LoadRange(_ context.Context, _ string, _, _ int) ([]session.Turn, error) {
	return nil, nil
}
func (s *stubStore) Close() error { return s.closeErr }

// stubDetector satisfies backend.LiveDetector for testing.
type stubDetector struct{ kind session.SourceKind }

func (d *stubDetector) Kind() session.SourceKind { return d.kind }
func (d *stubDetector) DetectLive(_ context.Context, _ backend.ProcessSnapshot) ([]backend.LiveSession, error) {
	return nil, nil
}

// stubEditor satisfies backend.SessionEditor for testing.
type stubEditor struct{ kind session.SourceKind }

func (e *stubEditor) Kind() session.SourceKind { return e.kind }
func (e *stubEditor) Prune(_ context.Context, _ string, _ backend.Selection) (backend.PruneResult, error) {
	return backend.PruneResult{}, nil
}
func (e *stubEditor) Delete(_ context.Context, _ string) error        { return nil }
func (e *stubEditor) RestoreBackup(_ context.Context, _ string) error { return nil }
func (e *stubEditor) HasBackup(_ string) bool                         { return false }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()
	store := &stubStore{kind: session.SourceClaude}
	det := &stubDetector{kind: session.SourceClaude}
	ed := &stubEditor{kind: session.SourceClaude}

	// Act
	reg.RegisterStore(store)
	reg.RegisterDetector(det)
	reg.RegisterEditor(ed)

	// Assert
	gotStore, ok := reg.Store(session.SourceClaude)
	require.True(t, ok)
	assert.Equal(t, store, gotStore)

	gotDet, ok := reg.Detector(session.SourceClaude)
	require.True(t, ok)
	assert.Equal(t, det, gotDet)

	gotEd, ok := reg.Editor(session.SourceClaude)
	require.True(t, ok)
	assert.Equal(t, ed, gotEd)
}

func TestRegistry_MissingKind_ReturnsFalse(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()

	// Act / Assert
	_, ok := reg.Store(session.SourceClaude)
	assert.False(t, ok)

	_, ok = reg.Detector(session.SourceClaude)
	assert.False(t, ok)

	_, ok = reg.Editor(session.SourceClaude)
	assert.False(t, ok)
}

func TestRegistry_Close_PropagatesErrors(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()
	errA := errors.New("store A error")
	errB := errors.New("store B error")
	reg.RegisterStore(&stubStore{kind: session.SourceClaude, closeErr: errA})
	reg.RegisterStore(&stubStore{kind: session.SourceOpenCode, closeErr: errB})

	// Act
	err := reg.Close()

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
}

func TestRegistry_Close_NoStores_ReturnsNil(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()

	// Act / Assert
	assert.NoError(t, reg.Close())
}

func TestRegistry_Stores_ReturnsAll(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()
	reg.RegisterStore(&stubStore{kind: session.SourceClaude})
	reg.RegisterStore(&stubStore{kind: session.SourceOpenCode})

	// Act
	stores := reg.Stores()

	// Assert
	assert.Len(t, stores, 2)
}

func TestRegistry_Detectors_ReturnsAll(t *testing.T) {
	// Arrange
	reg := backend.NewRegistry()
	reg.RegisterDetector(&stubDetector{kind: session.SourceClaude})
	reg.RegisterDetector(&stubDetector{kind: session.SourceOpenCode})

	// Act
	dets := reg.Detectors()

	// Assert
	assert.Len(t, dets, 2)
}
