package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds user preferences loaded from ~/.agentlens/config.json.
// See SPEC §4.3. Unknown fields are ignored on load and dropped on save.
type Config struct {
	Theme                string   `json:"theme"`
	GapThresholdSeconds  int      `json:"gap_threshold_seconds"`
	SessionDirs          []string `json:"session_dirs"`
	DefaultContextWindow int      `json:"default_context_window"`
	SchemaVersion        int      `json:"schema_version"`
}

// Default returns a Config populated with v1 defaults (SPEC §4.3).
func Default() Config {
	return Config{
		Theme:                "catppuccin",
		GapThresholdSeconds:  3 * 60,
		SessionDirs:          nil,
		DefaultContextWindow: 200_000,
		SchemaVersion:        1,
	}
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

// Path returns the absolute path to ~/.agentlens/config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file, filling any missing fields from Default().
// A missing file is not an error — defaults are returned.
func Load() (Config, error) {
	cfg := Default()
	p, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("config parse failed — using defaults", "path", p, "err", err)
		return Default(), nil
	}
	// Fill any zero-valued fields back from defaults.
	d := Default()
	if cfg.Theme == "" {
		cfg.Theme = d.Theme
	}
	if cfg.GapThresholdSeconds == 0 {
		cfg.GapThresholdSeconds = d.GapThresholdSeconds
	}
	if cfg.DefaultContextWindow == 0 {
		cfg.DefaultContextWindow = d.DefaultContextWindow
	}
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = d.SchemaVersion
	}
	return cfg, nil
}

// Save writes cfg to ~/.agentlens/config.json, creating the directory if
// needed. Writes via tmpfile+rename for atomicity.
func Save(cfg Config) error {
	if _, err := EnsureDir(); err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}
