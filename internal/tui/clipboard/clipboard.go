// Package clipboard provides a platform-gated clipboard copy helper.
package clipboard

import "errors"

// ErrNoClipboardTool is returned when no clipboard tool is found.
var ErrNoClipboardTool = errors.New("no clipboard tool available")

// Copy writes s to the system clipboard.
// On macOS it uses pbcopy; on Linux it tries wl-copy then xclip.
func Copy(s string) error {
	return platformCopy(s)
}
