package claude

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Sidecar holds the live-session metadata written by Claude Code to
// ~/.claude/sessions/<pid>.json.
type Sidecar struct {
	PID       int       `json:"pid"`
	SessionID string    `json:"sessionId"`
	CWD       string    `json:"cwd"`
	StartedAt time.Time `json:"startedAt"`
}

// ReadSidecars globs dir for *.json files and decodes each as a Sidecar.
// Decode failures are logged at warn and skipped.
func ReadSidecars(dir string) ([]Sidecar, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}

	var out []Sidecar
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			slog.Warn("open sidecar", "path", path, "err", err)
			continue
		}
		sc, err := decodeSidecar(f)
		_ = f.Close()
		if err != nil {
			slog.Warn("decode sidecar", "path", path, "err", err)
			continue
		}
		out = append(out, sc)
	}
	return out, nil
}

// decodeSidecar decodes a single sidecar JSON from r.
func decodeSidecar(r io.Reader) (Sidecar, error) {
	var sc Sidecar
	if err := json.NewDecoder(r).Decode(&sc); err != nil {
		return Sidecar{}, err
	}
	return sc, nil
}
