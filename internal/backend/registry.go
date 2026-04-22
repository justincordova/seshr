package backend

import (
	"errors"

	"github.com/justincordova/seshr/internal/session"
)

// Registry maps SourceKind values to backend implementations.
// No global singleton — the TUI app owns one Registry.
type Registry struct {
	stores    map[session.SourceKind]SessionStore
	detectors map[session.SourceKind]LiveDetector
	editors   map[session.SourceKind]SessionEditor
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		stores:    make(map[session.SourceKind]SessionStore),
		detectors: make(map[session.SourceKind]LiveDetector),
		editors:   make(map[session.SourceKind]SessionEditor),
	}
}

// RegisterStore registers a SessionStore implementation.
func (r *Registry) RegisterStore(s SessionStore) {
	r.stores[s.Kind()] = s
}

// RegisterDetector registers a LiveDetector implementation.
func (r *Registry) RegisterDetector(d LiveDetector) {
	r.detectors[d.Kind()] = d
}

// RegisterEditor registers a SessionEditor implementation.
func (r *Registry) RegisterEditor(e SessionEditor) {
	r.editors[e.Kind()] = e
}

// Store returns the SessionStore for the given kind, or (nil, false).
func (r *Registry) Store(kind session.SourceKind) (SessionStore, bool) {
	s, ok := r.stores[kind]
	return s, ok
}

// Detector returns the LiveDetector for the given kind, or (nil, false).
func (r *Registry) Detector(kind session.SourceKind) (LiveDetector, bool) {
	d, ok := r.detectors[kind]
	return d, ok
}

// Editor returns the SessionEditor for the given kind, or (nil, false).
func (r *Registry) Editor(kind session.SourceKind) (SessionEditor, bool) {
	e, ok := r.editors[kind]
	return e, ok
}

// Stores returns all registered SessionStore implementations.
func (r *Registry) Stores() []SessionStore {
	out := make([]SessionStore, 0, len(r.stores))
	for _, s := range r.stores {
		out = append(out, s)
	}
	return out
}

// Detectors returns all registered LiveDetector implementations.
func (r *Registry) Detectors() []LiveDetector {
	out := make([]LiveDetector, 0, len(r.detectors))
	for _, d := range r.detectors {
		out = append(out, d)
	}
	return out
}

// Close closes every registered SessionStore, collecting all errors.
func (r *Registry) Close() error {
	var errs []error
	for _, s := range r.stores {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
