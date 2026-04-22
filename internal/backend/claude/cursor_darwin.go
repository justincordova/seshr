//go:build darwin

package claude

import (
	"fmt"
	"os"
)

// fileIdentity reads Darwin-compatible identity fields (size + mtime).
func fileIdentity(path string) (cursorData, error) {
	info, err := os.Stat(path)
	if err != nil {
		return cursorData{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return cursorData{
		MtimeNs:   info.ModTime().UnixNano(),
		SizeBytes: info.Size(),
	}, nil
}
