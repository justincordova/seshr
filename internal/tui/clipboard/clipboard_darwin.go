//go:build darwin

package clipboard

import (
	"bytes"
	"fmt"
	"os/exec"
)

func platformCopy(s string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = bytes.NewBufferString(s)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pbcopy: %w", err)
	}
	return nil
}
