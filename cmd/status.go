package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the real-time multi-agent coordination dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not inside an initialized teamlead repository. Run 'teamlead init' first.")
			return errors.New("uninitialized repository")
		}
		return ui.RenderDashboard(root)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
