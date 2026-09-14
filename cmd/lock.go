package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	lockAcqAgent   string
	lockAcqTaskID  string
	lockAcqReason  string
	lockAcqTTL     int
	lockRelAgent   string
	lockRelAllAgent string
	lockCheckAgent string
)

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Manage file leases, locks, and collision prevention",
}

var lockAcquireCmd = &cobra.Command{
	Use:   "acquire <paths...>",
	Short: "Acquire exclusive lease on files or glob patterns",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		if lockAcqAgent == "" {
			ui.Error("Agent is required. Use --agent <name>.")
			return errors.New("missing agent flag")
		}

		var taskID *string
		if lockAcqTaskID != "" {
			taskID = &lockAcqTaskID
		}
		var reason *string
		if lockAcqReason != "" {
			reason = &lockAcqReason
		}
		var ttl *int
		if lockAcqTTL > 0 {
			ttl = &lockAcqTTL
		}

		ok, acquired, conflicts, err := lock.AcquireLocks(root, args, lockAcqAgent, taskID, reason, ttl)
		if err != nil {
			return err
		}

		if ok {
			var pathList []string
			for _, l := range acquired {
				pathList = append(pathList, l.Path)
			}
			ui.Successf("Acquired locks for agent '%s' on: %s", lockAcqAgent, strings.Join(pathList, ", "))
			return nil
		}

		for _, c := range conflicts {
			ui.Errorf("Conflict: '%s' is already locked by '%s' (expires in %dm)", c.Path, c.Agent, int(c.RemainingMinutes()))
		}
		return errors.New("lock conflict")
	},
}

var lockReleaseCmd = &cobra.Command{
	Use:   "release <paths...>",
	Short: "Release specific file locks held by an agent",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		if lockRelAgent == "" {
			ui.Error("Agent is required. Use --agent <name>.")
			return errors.New("missing agent flag")
		}

		released, err := lock.ReleaseLocks(root, args, lockRelAgent)
		if err != nil {
			return err
		}

		if len(released) > 0 {
			ui.Successf("Released locks: %s", strings.Join(released, ", "))
		} else {
			ui.Info("No matching locks were found for this agent.")
		}
		return nil
	},
}

var lockReleaseAllCmd = &cobra.Command{
	Use:   "release-all",
	Short: "Release all locks held by an agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		if lockRelAllAgent == "" {
			ui.Error("Agent is required. Use --agent <name>.")
			return errors.New("missing agent flag")
		}

		count, err := lock.ReleaseAllForAgent(root, lockRelAllAgent)
		if err != nil {
			return err
		}

		ui.Successf("Released %d lock(s) for agent '%s'.", count, lockRelAllAgent)
		return nil
	},
}

var lockCheckCmd = &cobra.Command{
	Use:   "check <path>",
	Short: "Check whether a file path or glob is safe to edit or locked",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		path := args[0]
		conflicts, err := lock.CheckConflicts(root, []string{path}, lockCheckAgent)
		if err != nil {
			return err
		}

		if len(conflicts) > 0 {
			for _, c := range conflicts {
				ui.Errorf("LOCKED: '%s' conflicts with '%s' held by '%s' (expires in %dm)", path, c.Path, c.Agent, int(c.RemainingMinutes()))
			}
			return errors.New("path is locked")
		}

		ui.Successf("FREE: '%s' is available to modify.", path)
		return nil
	},
}

var lockListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active file leases",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		locks, err := lock.GetActiveLocks(root, true)
		if err != nil {
			return err
		}

		if len(locks) == 0 {
			ui.Info("No active file locks.")
			return nil
		}

		for _, l := range locks {
			fmt.Printf("- %s -> %s (TTL: %dm)\n", l.Path, l.Agent, int(l.RemainingMinutes()))
		}
		return nil
	},
}

func init() {
	lockAcquireCmd.Flags().StringVarP(&lockAcqAgent, "agent", "a", "", "Agent acquiring the lock (required)")
	lockAcquireCmd.Flags().StringVarP(&lockAcqTaskID, "task", "t", "", "Associated task ID")
	lockAcquireCmd.Flags().StringVarP(&lockAcqReason, "reason", "r", "", "Reason or feature description")
	lockAcquireCmd.Flags().IntVar(&lockAcqTTL, "ttl", 0, "Lease time-to-live in minutes")

	lockReleaseCmd.Flags().StringVarP(&lockRelAgent, "agent", "a", "", "Agent holding the lock (required)")

	lockReleaseAllCmd.Flags().StringVarP(&lockRelAllAgent, "agent", "a", "", "Agent holding the locks (required)")

	lockCheckCmd.Flags().StringVarP(&lockCheckAgent, "agent", "a", "", "Agent asking (ignores locks held by self)")

	lockCmd.AddCommand(lockAcquireCmd)
	lockCmd.AddCommand(lockReleaseCmd)
	lockCmd.AddCommand(lockReleaseAllCmd)
	lockCmd.AddCommand(lockCheckCmd)
	lockCmd.AddCommand(lockListCmd)

	rootCmd.AddCommand(lockCmd)
}
