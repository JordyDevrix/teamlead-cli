package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var monitorInterval int

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Continuously monitor active agents, locks, and conflicts in real-time",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not inside an initialized teamlead repository. Run 'teamlead init' first.")
			return errors.New("uninitialized repository")
		}

		if monitorInterval <= 0 {
			monitorInterval = 2
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		ui.Info("Starting teamlead live monitor (Press Ctrl+C to stop)...")
		ticker := time.NewTicker(time.Duration(monitorInterval) * time.Second)
		defer ticker.Stop()

		// Initial render
		fmt.Print("\033[H\033[2J")
		_ = ui.RenderDashboard(root)

		for {
			select {
			case <-sigChan:
				ui.Info("Stopped monitor.")
				return nil
			case <-ticker.C:
				fmt.Print("\033[H\033[2J")
				_ = ui.RenderDashboard(root)
			}
		}
	},
}

func init() {
	monitorCmd.Flags().IntVarP(&monitorInterval, "interval", "i", 2, "Refresh interval in seconds")
	rootCmd.AddCommand(monitorCmd)
}
