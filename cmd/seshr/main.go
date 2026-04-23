package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/backend"
	claudeBackend "github.com/justincordova/seshr/internal/backend/claude"
	ocBackend "github.com/justincordova/seshr/internal/backend/opencode"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/logging"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/justincordova/seshr/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	var (
		debug          bool
		showVersion    bool
		noLive         bool
		dirOverride    string
		themeOverride  string
		opencodeDBPath string
	)

	root := &cobra.Command{
		Use:           "seshr [session-path]",
		Short:         "Replay and edit AI agent conversation sessions",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println(version.Version)
				return nil
			}
			if err := logging.Init(debug); err != nil {
				return fmt.Errorf("init logger: %w", err)
			}
			defer logging.Close()
			slog.Info("seshr starting", "version", version.Version, "debug", debug)

			cfg, err := config.Load()
			if err != nil {
				slog.Warn("config load failed — using defaults", "err", err)
				cfg = config.Default()
			}
			if themeOverride != "" {
				cfg.Theme = themeOverride
			}
			slog.Info("config loaded", "theme", cfg.Theme)

			scanRoot := dirOverride
			if scanRoot == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("user home: %w", err)
				}
				scanRoot = filepath.Join(home, ".claude", "projects")
			}

			if len(args) == 1 {
				slog.Info("positional path arg ignored", "path", args[0])
			}

			reg := backend.NewRegistry()
			claudeStore := claudeBackend.NewStore(scanRoot)
			reg.RegisterStore(claudeStore)
			reg.RegisterEditor(claudeBackend.NewEditor(claudeStore))
			if !noLive {
				home, _ := os.UserHomeDir()
				sidecarDir := filepath.Join(home, ".claude", "sessions")
				reg.RegisterDetector(claudeBackend.NewDetector(scanRoot, sidecarDir))
			}

			// OpenCode is registered only when its DB exists. Absence is not
			// an error — user simply doesn't have OC installed.
			ocPath := opencodeDBPath
			if ocPath == "" {
				p, err := ocBackend.DefaultDBPath()
				if err == nil {
					ocPath = p
				}
			}
			seshrDataDir, _ := os.UserHomeDir()
			ocBackupDir := filepath.Join(seshrDataDir, ".seshr", "backups", "opencode")
			ocStore, ocErr := ocBackend.NewStore(ocPath, ocBackupDir)
			switch {
			case ocErr == nil:
				reg.RegisterStore(ocStore)
				reg.RegisterEditor(ocBackend.NewEditor(ocStore, ocBackupDir))
				if !noLive {
					reg.RegisterDetector(ocBackend.NewDetector(ocStore))
				}
				slog.Info("opencode backend registered", "db", ocPath)
			case errors.Is(ocErr, ocBackend.ErrNoDatabase):
				slog.Debug("opencode backend skipped — no database found", "path", ocPath)
			default:
				slog.Warn("opencode backend disabled due to error", "err", ocErr)
			}

			defer func() { _ = reg.Close() }()

			metas, err := scanAllStores(context.Background(), reg)
			if err != nil {
				slog.Error("scan sessions", "err", err)
				return fmt.Errorf("scan sessions: %w", err)
			}
			slog.Info("scanned sessions", "count", len(metas))

			p := tea.NewProgram(tui.NewApp(metas, cfg, scanRoot, reg, noLive), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				slog.Error("tui exited with error", "err", err)
				return fmt.Errorf("run tui: %w", err)
			}
			slog.Info("seshr exiting")
			return nil
		},
	}

	root.Flags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.Flags().BoolVar(&showVersion, "version", false, "print version and exit")
	root.Flags().BoolVar(&noLive, "no-live", false, "disable live session detection")
	root.Flags().StringVar(&dirOverride, "dir", "", "directory to scan for Claude sessions (default ~/.claude/projects)")
	root.Flags().StringVar(&themeOverride, "theme", "", "color theme: catppuccin-mocha, nord, dracula")
	root.Flags().StringVar(&opencodeDBPath, "opencode-db", "", "path to OpenCode SQLite DB (default ~/.local/share/opencode/opencode.db)")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// scanAllStores calls Scan on every registered SessionStore and merges the
// results. A failure in any one store logs a warn and continues — losing
// Claude sessions because OC can't read its DB (or vice versa) is worse
// than a partial picker.
func scanAllStores(ctx context.Context, reg *backend.Registry) ([]backend.SessionMeta, error) {
	var out []backend.SessionMeta
	for _, s := range reg.Stores() {
		metas, err := s.Scan(ctx)
		if err != nil {
			slog.Warn("store scan failed; continuing", "kind", s.Kind(), "err", err)
			continue
		}
		slog.Info("store scan complete", "kind", s.Kind(), "count", len(metas))
		out = append(out, metas...)
	}
	return out, nil
}
