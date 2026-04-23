package claude

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justincordova/seshr/internal/backend"
	"github.com/justincordova/seshr/internal/session"
)

// Detector implements backend.LiveDetector for Claude Code.
type Detector struct {
	projectsDir  string
	sidecarDir   string
	readSidecars func(dir string) ([]Sidecar, error)
	now          func() time.Time
}

// NewDetector returns a Detector for the given projects and sidecar directories.
func NewDetector(projectsDir, sidecarDir string) *Detector {
	return &Detector{
		projectsDir:  projectsDir,
		sidecarDir:   sidecarDir,
		readSidecars: ReadSidecars,
		now:          time.Now,
	}
}

func (d *Detector) Kind() session.SourceKind { return session.SourceClaude }

// DetectLive implements backend.LiveDetector.
// Layer 1 (sidecar + PID match) runs first; unmatched PIDs fall through to
// layer 2 (cwd fallback via transcript mtime).
func (d *Detector) DetectLive(_ context.Context, snap backend.ProcessSnapshot) ([]backend.LiveSession, error) {
	claudePIDs := filterClaudePIDs(snap)

	// Layer 1: sidecar-based detection.
	layer1, unmatched := d.detectViaSidecar(snap, claudePIDs)

	// Layer 2: cwd fallback for unmatched PIDs.
	layer2 := d.detectViaCWD(snap, unmatched)

	return append(layer1, layer2...), nil
}

// filterClaudePIDs returns PIDs whose command is a claude agent (not --print).
func filterClaudePIDs(snap backend.ProcessSnapshot) map[int]backend.ProcInfo {
	out := make(map[int]backend.ProcInfo)
	for pid, proc := range snap.ByPID {
		cmd := proc.Command
		tokens := strings.Fields(cmd)
		isClaude := false
		for i := 0; i < 2 && i < len(tokens); i++ {
			base := filepath.Base(tokens[i])
			if strings.Contains(base, "claude") {
				isClaude = true
				break
			}
		}
		if !isClaude {
			continue
		}
		// Skip non-interactive mode.
		if strings.Contains(cmd, "--print") {
			continue
		}
		out[pid] = proc
	}
	return out
}

// detectViaSidecar runs layer 1. Returns matched LiveSessions plus the set of
// claude PIDs that did NOT match any sidecar.
func (d *Detector) detectViaSidecar(snap backend.ProcessSnapshot, claudePIDs map[int]backend.ProcInfo) ([]backend.LiveSession, map[int]backend.ProcInfo) {
	sidecars, err := d.readSidecars(d.sidecarDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("read sidecars", "dir", d.sidecarDir, "err", err)
	}

	matched := make(map[int]bool)
	var out []backend.LiveSession

	for _, sc := range sidecars {
		proc, ok := claudePIDs[sc.PID]
		if !ok {
			continue
		}
		matched[sc.PID] = true

		// Find the transcript path for status derivation.
		transcriptPath := d.transcriptForSession(sc.SessionID)
		status := deriveStatus(transcriptPath, proc, childProcs(snap, sc.PID), d.now())
		currentTask := extractCurrentTask(transcriptPath)

		out = append(out, backend.LiveSession{
			SessionID:    sc.SessionID,
			Kind:         session.SourceClaude,
			PID:          sc.PID,
			CWD:          sc.CWD,
			Project:      encodeCWDToProjectDir(sc.CWD),
			Status:       status,
			CurrentTask:  currentTask,
			LastActivity: lastMtime(transcriptPath, d.now()),
			Ambiguous:    false,
		})
	}

	// Return unmatched claude PIDs for layer 2.
	unmatched := make(map[int]backend.ProcInfo)
	for pid, proc := range claudePIDs {
		if !matched[pid] {
			unmatched[pid] = proc
		}
	}
	return out, unmatched
}

// detectViaCWD runs layer 2: cwd-based transcript lookup for unmatched PIDs.
func (d *Detector) detectViaCWD(snap backend.ProcessSnapshot, unmatched map[int]backend.ProcInfo) []backend.LiveSession {
	var out []backend.LiveSession
	seen := make(map[string]bool)

	for pid, proc := range unmatched {
		if proc.CWD == "" {
			continue
		}
		encoded := encodeCWDToProjectDir(proc.CWD)
		pattern := filepath.Join(d.projectsDir, encoded, "*.jsonl")
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			continue
		}

		// Find most-recently-modified transcript.
		newest := newestFile(matches)
		if newest == "" {
			continue
		}

		info, err := os.Stat(newest)
		if err != nil {
			continue
		}
		if d.now().Sub(info.ModTime()) > 5*time.Minute {
			continue
		}

		sessionID := strings.TrimSuffix(filepath.Base(newest), ".jsonl")
		if seen[sessionID] {
			continue
		}
		seen[sessionID] = true

		status := deriveStatus(newest, proc, childProcs(snap, pid), d.now())
		currentTask := extractCurrentTask(newest)

		out = append(out, backend.LiveSession{
			SessionID:    sessionID,
			Kind:         session.SourceClaude,
			PID:          pid,
			CWD:          proc.CWD,
			Project:      encoded,
			Status:       status,
			CurrentTask:  currentTask,
			LastActivity: info.ModTime(),
			Ambiguous:    false,
		})
	}
	return out
}

// EncodeCWDToProjectDir encodes a CWD path into Claude's project dir format.
// e.g. /Users/foo/bar.v2 → -Users-foo-bar-v2
//
// Exported so the TUI can derive a Project name when synthesizing a
// SessionMeta for a live session that hasn't produced a transcript file yet.
func EncodeCWDToProjectDir(cwd string) string {
	return encodeCWDToProjectDir(cwd)
}

// encodeCWDToProjectDir is the internal implementation.
func encodeCWDToProjectDir(cwd string) string {
	r := strings.NewReplacer("/", "-", "_", "-", ".", "-")
	result := r.Replace(cwd)
	// Remove leading dash if present (from leading /).
	result = strings.TrimLeft(result, "-")
	// Claude's encoding starts with a dash: re-prefix.
	if cwd != "" && cwd[0] == '/' {
		result = "-" + result
	}
	return result
}

// deriveStatus computes Working vs Waiting based on transcript mtime and CPU.
func deriveStatus(transcriptPath string, proc backend.ProcInfo, children []backend.ProcInfo, now time.Time) backend.Status {
	if transcriptPath != "" {
		if info, err := os.Stat(transcriptPath); err == nil {
			if now.Sub(info.ModTime()) < 30*time.Second {
				return backend.StatusWorking
			}
		}
	}
	if proc.CPU > 1.0 {
		return backend.StatusWorking
	}
	for _, child := range children {
		if child.CPU > 5.0 {
			return backend.StatusWorking
		}
	}
	return backend.StatusWaiting
}

// extractCurrentTask tails a JSONL file and returns the last tool_use name+arg.
// Returns "" if not found.
func extractCurrentTask(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	// Read last 64KB.
	const window = 64 * 1024
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	offset := info.Size() - window
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}

	return extractLastToolUse(f)
}

// transcriptForSession locates the .jsonl for a session ID under projectsDir.
func (d *Detector) transcriptForSession(sessionID string) string {
	pattern := filepath.Join(d.projectsDir, "*", sessionID+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// childProcs returns the direct child ProcInfos of pid from the snapshot.
func childProcs(snap backend.ProcessSnapshot, pid int) []backend.ProcInfo {
	var out []backend.ProcInfo
	for _, childPID := range snap.Children[pid] {
		if proc, ok := snap.ByPID[childPID]; ok {
			out = append(out, proc)
		}
	}
	return out
}

// newestFile returns the path with the most-recent mtime from candidates.
func newestFile(paths []string) string {
	var newest string
	var newestTime time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newest = p
		}
	}
	return newest
}

// lastMtime returns the mtime of path, or fallback if stat fails.
func lastMtime(path string, fallback time.Time) time.Time {
	if path == "" {
		return fallback
	}
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.ModTime()
}
