package backend

import (
	"context"
	"time"

	"github.com/justincordova/seshr/internal/session"
)

// SessionStore reads session metadata and turns from a source.
type SessionStore interface {
	Kind() session.SourceKind
	Scan(ctx context.Context) ([]SessionMeta, error)
	Load(ctx context.Context, id string) (*session.Session, Cursor, error)
	LoadIncremental(ctx context.Context, id string, cur Cursor) ([]session.Turn, Cursor, error)
	LoadRange(ctx context.Context, id string, fromIdx, toIdx int) ([]session.Turn, error)
	Close() error
}

// LiveDetector detects running agent processes and maps them to sessions.
type LiveDetector interface {
	Kind() session.SourceKind
	DetectLive(ctx context.Context, procs ProcessSnapshot) ([]LiveSession, error)
}

// SessionEditor handles pruning, deletion, and restore for a given source.
type SessionEditor interface {
	Kind() session.SourceKind
	Prune(ctx context.Context, id string, sel Selection) (PruneResult, error)
	Delete(ctx context.Context, id string) error
	RestoreBackup(ctx context.Context, id string) error
	HasBackup(id string) bool
}

// Cursor carries opaque per-source incremental load state.
// Kind tags which backend owns the cursor; Data is source-specific JSON.
type Cursor struct {
	Kind session.SourceKind
	Data []byte
}

// SessionMeta is lightweight session metadata returned by Scan.
type SessionMeta struct {
	ID         string
	Kind       session.SourceKind
	Project    string
	Directory  string
	Title      string
	TokenCount int
	TurnCount  int
	CostUSD    float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	SizeBytes  int64
	HasBackup  bool
}

// LiveSession describes a running agent process mapped to a session.
//
// CWD is the agent process's working directory at detection time. Project
// is the source-specific project name (e.g. Claude's encoded project-dir
// name). Both are populated by the detector so the TUI can synthesize a
// SessionMeta for live sessions that haven't produced a transcript yet
// (newly-opened Claude session, OC session with no messages). Empty when
// the detector couldn't determine the value.
type LiveSession struct {
	SessionID     string
	Kind          session.SourceKind
	PID           int
	CWD           string
	Project       string
	Status        Status
	CurrentTask   string
	LastActivity  time.Time
	ContextTokens int
	ContextWindow int
	TokenCount    int
	TurnCount     int
	Ambiguous     bool
}

// Status classifies a live agent session.
type Status int

const (
	StatusWaiting Status = iota
	StatusWorking
)

// PruneResult is returned by SessionEditor.Prune. Claude always returns the
// zero value; the SkippedRunningTools field is populated by the OpenCode
// editor (Phase 11).
type PruneResult struct {
	SkippedRunningTools int
}
