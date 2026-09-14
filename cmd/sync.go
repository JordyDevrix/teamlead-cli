package cmd

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	syncAgentName string
	syncStrategy  string
)

var syncCmd = &cobra.Command{
	Use:   "sync --agent <name>",
	Short: "Rebase or sync an agent's worktree branch against the latest base branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		if syncAgentName == "" {
			ui.Error("Agent is required. Use --agent <name>.")
			return errors.New("missing agent flag")
		}

		cfg, err := config.LoadConfig(root)
		if err != nil {
			return err
		}

		ag, err := agent.GetAgent(root, syncAgentName)
		if err != nil {
			return err
		}
		if ag == nil || ag.WorktreePath == nil || *ag.WorktreePath == "" {
			ui.Errorf("No active worktree found for agent '%s'.", syncAgentName)
			return errors.New("worktree not found")
		}

		ok, msg, err := git.SyncWorktreeBranch(*ag.WorktreePath, cfg.BaseBranch, syncStrategy)
		if err != nil {
			return err
		}

		if ok {
			ui.Success(msg)
			return nil
		}

		ui.Error(msg)
		return errors.New(msg)
	},
}

func init() {
	syncCmd.Flags().StringVarP(&syncAgentName, "agent", "a", "", "Agent whose worktree to sync with base branch (required)")
	syncCmd.Flags().StringVar(&syncStrategy, "strategy", "rebase", "Sync strategy (rebase or merge)")

	rootCmd.AddCommand(syncCmd)
}
