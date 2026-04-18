package tui

import (
	"context"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/topics"
)

// SessionLoadedMsg is emitted when a session has been parsed and clustered.
type SessionLoadedMsg struct {
	Session *parser.Session
	Topics  []topics.Topic
}

// SessionLoadErrMsg is emitted when parsing or clustering fails.
type SessionLoadErrMsg struct {
	Path string
	Err  error
}

// LoadSessionCmd returns a tea.Cmd that parses and clusters the session at path.
func LoadSessionCmd(path string) tea.Cmd {
	return func() tea.Msg {
		p := parser.NewClaude()
		sess, err := p.Parse(context.Background(), path)
		if err != nil {
			slog.Error("load session failed", "path", path, "err", err)
			return SessionLoadErrMsg{Path: path, Err: err}
		}
		tops := topics.Cluster(sess, topics.DefaultOptions())
		slog.Info("clustered session", "path", path, "turns", len(sess.Turns), "topics", len(tops))
		return SessionLoadedMsg{Session: sess, Topics: tops}
	}
}
