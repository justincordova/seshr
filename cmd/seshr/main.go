package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/seshr/internal/config"
	"github.com/justincordova/seshr/internal/logging"
	"github.com/justincordova/seshr/internal/parser"
	"github.com/justincordova/seshr/internal/tui"
	"github.com/justincordova/seshr/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	var (
		debug         bool
		showVersion   bool
		dirOverride   string
		themeOverride string
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

			metas, err := parser.Scan(scanRoot)
			if err != nil {
				slog.Error("scan sessions", "root", scanRoot, "err", err)
				return fmt.Errorf("scan %s: %w", scanRoot, err)
			}
			slog.Info("scanned sessions", "root", scanRoot, "count", len(metas))

			if len(args) == 1 {
				slog.Info("positional path arg ignored in phase 2", "path", args[0])
			}

			p := tea.NewProgram(tui.NewApp(metas, cfg), tea.WithAltScreen())
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
	root.Flags().StringVar(&dirOverride, "dir", "", "directory to scan for sessions (default ~/.claude/projects)")
	root.Flags().StringVar(&themeOverride, "theme", "", "color theme: catppuccin-mocha, nord, dracula")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
