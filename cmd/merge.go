package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/merge"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	mergeStrategy     string
	mergeNoTest       bool
	mergeKeepBranch   bool
	mergeKeepWorktree bool
)

var mergeCmd = &cobra.Command{
	Use:   "merge <agent>",
	Short: "Validate, test, and cleanly merge an agent's work into the base branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		agentName := args[0]
		ui.Infof("Preparing to integrate work from agent '%s'...", agentName)

		var strat *string
		if mergeStrategy != "" {
			strat = &mergeStrategy
		}

		ok, msg, err := merge.MergeAgentWork(
			root,
			agentName,
			strat,
			!mergeKeepBranch,
			!mergeKeepWorktree,
			!mergeNoTest,
		)
		if err != nil {
			return err
		}

		if ok {
			ui.Success(msg)
			return nil
		}

		ui.Errorf("Integration failed: %s", msg)
		return errors.New(msg)
	},
}

func init() {
	mergeCmd.Flags().StringVar(&mergeStrategy, "strategy", "", "Merge strategy (squash, rebase, merge)")
	mergeCmd.Flags().BoolVar(&mergeNoTest, "no-test", false, "Skip pre-merge validation tests")
	mergeCmd.Flags().BoolVar(&mergeKeepBranch, "keep-branch", false, "Keep the feature branch after merging")
	mergeCmd.Flags().BoolVar(&mergeKeepWorktree, "keep-worktree", false, "Keep the agent's worktree folder")

	rootCmd.AddCommand(mergeCmd)
}
