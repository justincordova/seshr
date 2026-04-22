//go:build linux

package claude

import (
	"fmt"
	"os"
	"syscall"
)

// fileIdentity reads Linux-compatible identity fields (inode + mtime).
func fileIdentity(path string) (cursorData, error) {
	info, err := os.Stat(path)
	if err != nil {
		return cursorData{}, fmt.Errorf("stat %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return cursorData{MtimeNs: info.ModTime().UnixNano(), SizeBytes: info.Size()}, nil
	}
	return cursorData{
		MtimeNs: info.ModTime().UnixNano(),
		Inode:   stat.Ino,
	}, nil
}
