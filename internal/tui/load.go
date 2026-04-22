package tui

import (
	"context"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// LoadSessionCmd returns a tea.Cmd that parses and clusters the session at path.
// gapSeconds configures the topic clustering time-gap threshold (0 uses default).
func LoadSessionCmd(path string, gapSeconds int) tea.Cmd {
	return func() tea.Msg {
		p := session.NewClaude()
		sess, err := p.Parse(context.Background(), path)
		if err != nil {
			slog.Error("load session failed", "path", path, "err", err)
			return SessionLoadErrMsg{Path: path, Err: err}
		}
		opts := topics.DefaultOptions()
		if gapSeconds > 0 {
			opts.GapThreshold = time.Duration(gapSeconds) * time.Second
		}
		tops := topics.Cluster(sess, opts)
		slog.Info("clustered session", "path", path, "turns", len(sess.Turns), "topics", len(tops))
		return SessionLoadedMsg{Session: sess, Topics: tops}
	}
}
