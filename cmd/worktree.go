package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/context"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	wtBranchName string
	wtBaseBranch string
	wtForceRemove bool
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage isolated git worktrees for agents",
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List git worktrees",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		wts, err := git.ListWorktrees(root)
		if err != nil {
			return err
		}

		for _, wt := range wts {
			branch := wt.Branch
			if branch == "" {
				branch = "detached"
			}
			fmt.Printf("- %s (%s)\n", wt.Path, branch)
		}
		return nil
	},
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <agent>",
	Short: "Create a dedicated worktree for an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		agName := args[0]
		cfg, err := config.LoadConfig(root)
		if err != nil {
			return err
		}

		wtPath := filepath.Join(root, cfg.WorktreeDir, agName)
		branch := wtBranchName
		if branch == "" {
			branch = fmt.Sprintf("teamlead/%s/workspace", agName)
		}
		base := wtBaseBranch
		if base == "" {
			base = cfg.BaseBranch
		}

		if err := git.CreateWorktree(root, wtPath, branch, base); err != nil {
			return fmt.Errorf("failed to create worktree: %w", err)
		}

		_, _ = agent.RegisterAgent(root, agName, nil, nil, &wtPath, &branch, nil)
		_, _ = context.InjectContextIntoWorktree(root, wtPath, agName)

		ui.Successf("Created worktree for '%s' at %s (branch: %s)", agName, wtPath, branch)
		return nil
	},
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <agent>",
	Short: "Remove an agent's worktree",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		agName := args[0]
		cfg, err := config.LoadConfig(root)
		if err != nil {
			return err
		}

		wtPath := filepath.Join(root, cfg.WorktreeDir, agName)
		if _, err := os.Stat(wtPath); err == nil {
			if err := git.RemoveWorktree(root, wtPath, wtForceRemove); err != nil {
				return err
			}
			ui.Successf("Removed worktree at %s", wtPath)
		} else {
			ui.Warningf("No worktree found at %s", wtPath)
		}
		return nil
	},
}

func init() {
	worktreeCreateCmd.Flags().StringVar(&wtBranchName, "branch", "", "Custom branch name")
	worktreeCreateCmd.Flags().StringVar(&wtBaseBranch, "base", "", "Base branch to branch off of")

	worktreeRemoveCmd.Flags().BoolVar(&wtForceRemove, "force", false, "Force removal even if uncommitted changes exist")

	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeCreateCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)

	rootCmd.AddCommand(worktreeCmd)
}
