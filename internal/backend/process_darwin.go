//go:build darwin

package backend

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// platformReadCWD uses lsof to look up the working directory of a process on macOS.
//
// The -a flag is critical: lsof's filter args (-p, -d) default to OR
// semantics, so without -a "lsof -p 23758 -d cwd" returns the cwd of every
// process on the system. -a forces AND.
func platformReadCWD(ctx context.Context, pid int) (string, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-a", "-p", fmt.Sprintf("%d", pid), "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("lsof pid %d: %w", pid, err)
	}
	return parseLsofCWD(out, pid)
}

// parseLsofCWD extracts the cwd path from lsof -Fn output, scoped to the
// given pid. -Fn output is a sequence of process blocks:
//
//	p<pid>
//	fcwd
//	n/path/to/cwd
//
// We track the current pid block and only return an n-line that belongs to
// it. With AND semantics from -a -p PID, only one block is expected, but
// scoping defends against future format changes or unrelated entries.
func parseLsofCWD(out []byte, wantPID int) (string, error) {
	var curPID int
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "p") {
			n, err := strconv.Atoi(line[1:])
			if err != nil {
				curPID = 0
				continue
			}
			curPID = n
			continue
		}
		if curPID != wantPID {
			continue
		}
		if strings.HasPrefix(line, "n") && len(line) > 1 && line[1] == '/' {
			return line[1:], nil
		}
	}
	return "", fmt.Errorf("no cwd for pid %d in lsof output", wantPID)
}
