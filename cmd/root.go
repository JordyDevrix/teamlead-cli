package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var rootCmd = &cobra.Command{
	Use:     "teamlead-cli",
	Aliases: []string{"teamlead"},
	Short:   "Conflict-free multi-agent coordinator and workspace isolation CLI",
	Long: `teamlead-cli allows multiple different AI agents, models, and CLI tools
(e.g. Claude Code, Aider, Cursor, Copilot, Gemini CLI, or custom scripts)
to collaborate concurrently on the same codebase without stepping on each other,
clobbering files, or breaking Git staging.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err == nil && config.IsInitialized(root) {
			return ui.RenderDashboard(root)
		}
		return cmd.Help()
	},
}

// Execute runs the root CLI command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = "1.0.0"
}
