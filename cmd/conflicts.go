package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/conflict"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var conflictsCmd = &cobra.Command{
	Use:   "conflicts",
	Short: "Inspect all active branches, worktrees, and locks for collisions",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		report, err := conflict.DetectConflicts(root)
		if err != nil {
			return err
		}

		if !report.HasBlockingConflicts && len(report.StaleBranchWarnings) == 0 && len(report.ScopeOverlapWarnings) == 0 {
			ui.Success("No conflicts or risks detected across active agents!")
			return nil
		}

		if len(report.DirectFileConflicts) > 0 {
			ui.Error("Direct File Collisions Detected:")
			for _, c := range report.DirectFileConflicts {
				fmt.Printf("  ✖ %s modified by both %s and %s\n", c.File, c.Agents[0], c.Agents[1])
			}
		}

		if len(report.LockViolations) > 0 {
			ui.Error("Lock Violations Detected:")
			for _, v := range report.LockViolations {
				fmt.Printf("  ✖ %s modified by %s but locked by %s\n", v.File, v.ModifyingAgent, v.LockingAgent)
			}
		}

		if len(report.StaleBranchWarnings) > 0 {
			ui.Warning("Stale Branch Warnings:")
			for _, s := range report.StaleBranchWarnings {
				fmt.Printf("  ⚠ Agent '%s' branch '%s' is %d commit(s) behind base branch.\n", s.Agent, s.Branch, s.Behind)
			}
		}

		if len(report.ScopeOverlapWarnings) > 0 {
			ui.Warning("Task Scope Overlaps:")
			for _, o := range report.ScopeOverlapWarnings {
				fmt.Printf("  ⚠ %s (%s) and %s (%s) both touch '%s'\n", o.Task1, o.Agent1, o.Task2, o.Agent2, o.Scope1)
			}
		}

		if len(report.Recommendations) > 0 {
			fmt.Println("\nActionable Recommendations:")
			for _, rec := range report.Recommendations {
				fmt.Printf("  💡 %s\n", rec)
			}
		}

		if report.HasBlockingConflicts {
			return errors.New("blocking conflicts detected")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(conflictsCmd)
}
