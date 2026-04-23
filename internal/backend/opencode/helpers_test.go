package opencode

import (
	"os"
)

// runtimeMkDirAll wraps os.MkdirAll to keep store_test.go free of `os`
// imports that the main test file doesn't otherwise need.
func runtimeMkDirAll(p string) error { return os.MkdirAll(p, 0o700) }

// writeFile writes content at p with user-only permissions. Used by tests
// that stage backup fixtures on disk.
func writeFile(p, content string) error {
	return os.WriteFile(p, []byte(content), 0o600)
}
