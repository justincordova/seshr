//go:build linux

package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func platformCopy(s string) error {
	// Try wl-copy first if Wayland is running.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if err := runCmd("wl-copy", s); err == nil {
			return nil
		}
	}
	// Fall back to xclip.
	if err := runCmd("xclip", s, "-selection", "clipboard"); err == nil {
		return nil
	}
	return ErrNoClipboardTool
}

func runCmd(name string, stdin string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewBufferString(stdin)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
