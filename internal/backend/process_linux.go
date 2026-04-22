//go:build linux

package backend

import (
	"context"
	"fmt"
	"os"
)

// platformReadCWD reads the working directory of a process via /proc on Linux.
func platformReadCWD(_ context.Context, pid int) (string, error) {
	path := fmt.Sprintf("/proc/%d/cwd", pid)
	cwd, err := os.Readlink(path)
	if err != nil {
		return "", fmt.Errorf("readlink %s: %w", path, err)
	}
	return cwd, nil
}
