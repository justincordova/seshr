package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/logging"
	"github.com/justincordova/agentlens/internal/parser"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/justincordova/agentlens/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	var (
		debug       bool
		showVersion bool
		dirOverride string
	)

	root := &cobra.Command{
		Use:           "agentlens [session-path]",
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
			slog.Info("agentlens starting", "version", version.Version, "debug", debug)

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

			p := tea.NewProgram(tui.NewApp(metas), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				slog.Error("tui exited with error", "err", err)
				return fmt.Errorf("run tui: %w", err)
			}
			slog.Info("agentlens exiting")
			return nil
		},
	}

	root.Flags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.Flags().BoolVar(&showVersion, "version", false, "print version and exit")
	root.Flags().StringVar(&dirOverride, "dir", "", "directory to scan for sessions (default ~/.claude/projects)")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
