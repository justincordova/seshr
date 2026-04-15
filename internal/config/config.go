package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds user preferences loaded from ~/.agentlens/config.json.
//
// TODO(phase-6): full schema per SPEC.md §4.3.
type Config struct {
	Theme string `json:"theme"`
}

// Dir returns the path to the agentlens data directory (~/.agentlens).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".agentlens"), nil
}

// EnsureDir creates ~/.agentlens with 0755 perms if it does not exist.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}
