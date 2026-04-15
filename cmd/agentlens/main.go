package main

import (
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justincordova/agentlens/internal/logging"
	"github.com/justincordova/agentlens/internal/tui"
	"github.com/justincordova/agentlens/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	var (
		debug       bool
		showVersion bool
	)

	root := &cobra.Command{
		Use:           "agentlens [session-path]",
		Short:         "Replay and edit AI agent conversation sessions",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Println(version.Version)
				return nil
			}
			if err := logging.Init(debug); err != nil {
				return fmt.Errorf("init logger: %w", err)
			}
			slog.Info("agentlens starting", "version", version.Version, "debug", debug)

			p := tea.NewProgram(tui.NewApp(), tea.WithAltScreen())
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

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
