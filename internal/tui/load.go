package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
	"github.com/justincordova/seshr/internal/topics"
)

// SessionLoadedMsg is emitted when a session has been parsed and clustered.
type SessionLoadedMsg struct {
	Session *session.Session
	Topics  []topics.Topic
}

// SessionLoadErrMsg is emitted when parsing or clustering fails.
type SessionLoadErrMsg struct {
	Path string
	Err  error
}

// LoadSessionByIDCmd loads a session via the backend registry and clusters it.
func LoadSessionByIDCmd(meta backend.SessionMeta, reg *backend.Registry, gapSeconds int) tea.Cmd {
	return func() tea.Msg {
		if reg == nil {
			return SessionLoadErrMsg{Path: meta.ID, Err: errNoRegistry}
		}
		store, ok := reg.Store(meta.Kind)
		if !ok {
			return SessionLoadErrMsg{Path: meta.ID, Err: errNoStore(meta.Kind)}
		}
		sess, _, err := store.Load(context.Background(), meta.ID)
		if err != nil {
			slog.Error("load session failed", "id", meta.ID, "err", err)
			return SessionLoadErrMsg{Path: meta.ID, Err: err}
		}
		opts := topics.DefaultOptions()
		if gapSeconds > 0 {
			opts.GapThreshold = time.Duration(gapSeconds) * time.Second
		}
		tops := topics.Cluster(sess, opts)
		slog.Info("clustered session", "id", meta.ID, "turns", len(sess.Turns), "topics", len(tops))
		return SessionLoadedMsg{Session: sess, Topics: tops}
	}
}

type errNoRegistryType struct{}

func (e errNoRegistryType) Error() string { return "no backend registry configured" }

var errNoRegistry = errNoRegistryType{}

type errNoStoreType struct{ kind session.SourceKind }

func (e errNoStoreType) Error() string { return "no store for kind: " + string(e.kind) }

func errNoStore(kind session.SourceKind) error { return errNoStoreType{kind: kind} }
