package backend

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// runPSDefault executes ps and returns raw output.
func runPSDefault(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ps", "-Ao", "pid=,ppid=,pcpu=,rss=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return out, nil
}

// parsePS parses ps output into a slice of ProcInfo.
// Each row must have at least 5 fields: pid ppid cpu rss command...
func parsePS(out []byte) ([]ProcInfo, error) {
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var procs []ProcInfo
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		// Collapse runs of spaces so SplitN works on any ps alignment.
		normalized := strings.Join(strings.Fields(line), " ")
		// Split into at most 5 parts: pid ppid cpu rss command-with-spaces.
		parts := strings.SplitN(normalized, " ", 5)
		if len(parts) < 5 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		cpu, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			continue
		}
		rss, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			continue
		}
		procs = append(procs, ProcInfo{
			PID:     pid,
			PPID:    ppid,
			CPU:     cpu,
			RSSKB:   rss,
			Command: parts[4],
		})
	}
	return procs, nil
}

// isAgentCandidate returns true when the command string suggests a Claude or
// OpenCode agent process — checked on the first two argv tokens (base name or
// full path may contain the agent name, e.g. node launchers for opencode).
func isAgentCandidate(command string) bool {
	tokens := strings.Fields(command)
	for i := 0; i < 2 && i < len(tokens); i++ {
		tok := tokens[i]
		// Check both the full token and the base name.
		if strings.Contains(tok, "claude") || strings.Contains(tok, "opencode") {
			return true
		}
		base := filepath.Base(tok)
		if strings.Contains(base, "claude") || strings.Contains(base, "opencode") {
			return true
		}
	}
	return false
}
