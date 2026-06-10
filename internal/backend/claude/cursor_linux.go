//go:build linux

package claude

import (
	"fmt"
	"os"
	"syscall"
)

// fileIdentity reads Linux-compatible identity fields (inode + size + mtime).
// Size matters even with an inode: an in-place truncation (as opposed to a
// rename-style replace) keeps the inode, and identitiesMatch must still be
// able to detect that the cursor's byte offset no longer points at a record
// boundary.
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
		MtimeNs:   info.ModTime().UnixNano(),
		SizeBytes: info.Size(),
		Inode:     stat.Ino,
	}, nil
}
