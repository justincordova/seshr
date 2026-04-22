//go:build darwin

package backend

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// platformReadCWD uses lsof to look up the working directory of a process on macOS.
func platformReadCWD(ctx context.Context, pid int) (string, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-p", fmt.Sprintf("%d", pid), "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("lsof pid %d: %w", pid, err)
	}
	return parseLsofCWD(out)
}

// parseLsofCWD extracts the cwd path from lsof -Fn output.
// Expected format (one or more blocks):
//
//	p<pid>
//	fcwd
//	n/path/to/cwd
func parseLsofCWD(out []byte) (string, error) {
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n/") || (strings.HasPrefix(line, "n") && len(line) > 1 && line[1] == '/') {
			return line[1:], nil
		}
	}
	return "", fmt.Errorf("no cwd found in lsof output")
}
