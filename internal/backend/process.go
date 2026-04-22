package backend

import (
	"context"
	"time"
)

// ProcessSnapshot is a point-in-time view of running processes.
type ProcessSnapshot struct {
	At       time.Time
	ByPID    map[int]ProcInfo
	Children map[int][]int // ppid → []pid
}

// ProcInfo describes a single OS process.
type ProcInfo struct {
	PID     int
	PPID    int
	Command string
	CPU     float64
	RSSKB   int64
	CWD     string // populated lazily, only for agent processes
}

// ProcessScanner produces ProcessSnapshots via ps + platform CWD lookup.
// The unexported fields are settable in tests (same package) for injection.
type ProcessScanner struct {
	now     func() time.Time
	runPS   func(ctx context.Context) ([]byte, error)
	readCWD func(ctx context.Context, pid int) (string, error)
}

// NewProcessScanner returns a scanner with production defaults.
func NewProcessScanner() *ProcessScanner {
	return &ProcessScanner{
		now:     time.Now,
		runPS:   runPSDefault,
		readCWD: platformReadCWD,
	}
}

// Scan runs ps + platform CWD lookup and returns a snapshot.
// If ps fails, the snapshot has At set and ByPID == nil.
func (p *ProcessScanner) Scan(ctx context.Context) (ProcessSnapshot, error) {
	out, err := p.runPS(ctx)
	if err != nil {
		return ProcessSnapshot{At: p.now()}, wrapErr("process scan", err)
	}

	procs, err := parsePS(out)
	if err != nil {
		return ProcessSnapshot{At: p.now()}, wrapErr("parse ps output", err)
	}

	byPID := make(map[int]ProcInfo, len(procs))
	children := make(map[int][]int)
	for _, pr := range procs {
		byPID[pr.PID] = pr
		children[pr.PPID] = append(children[pr.PPID], pr.PID)
	}

	// Populate CWD only for agent candidates.
	for pid, pr := range byPID {
		if isAgentCandidate(pr.Command) {
			cwd, err := p.readCWD(ctx, pid)
			if err != nil {
				// Silently ignore per-PID CWD errors.
				continue
			}
			pr.CWD = cwd
			byPID[pid] = pr
		}
	}

	return ProcessSnapshot{
		At:       p.now(),
		ByPID:    byPID,
		Children: children,
	}, nil
}

func wrapErr(msg string, err error) error {
	if err == nil {
		return nil
	}
	return &scanError{msg: msg, cause: err}
}

type scanError struct {
	msg   string
	cause error
}

func (e *scanError) Error() string { return e.msg + ": " + e.cause.Error() }
func (e *scanError) Unwrap() error { return e.cause }
