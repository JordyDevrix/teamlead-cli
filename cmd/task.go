package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	taskDesc        string
	taskScope       []string
	taskStatusFilter string
	taskAgentFilter  string
	taskClaimAgent  string
	taskCompAgent   string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks, backlogs, and file scope reservations",
}

var taskAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a new task with declared file scopes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		title := args[0]
		var desc *string
		if taskDesc != "" {
			desc = &taskDesc
		}

		t, err := task.CreateTask(root, title, desc, taskScope)
		if err != nil {
			return err
		}

		ui.Successf("Created task %s: %s", t.ID, t.Title)
		if len(t.Scope) > 0 {
			ui.Infof("Reserved scope: %s", strings.Join(t.Scope, ", "))
		}
		return nil
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks in the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		var stFilter *string
		if taskStatusFilter != "" {
			stFilter = &taskStatusFilter
		}
		var agFilter *string
		if taskAgentFilter != "" {
			agFilter = &taskAgentFilter
		}

		tasks, err := task.ListTasks(root, stFilter, agFilter)
		if err != nil {
			return err
		}

		if len(tasks) == 0 {
			ui.Info("No tasks matching criteria.")
			return nil
		}

		for _, t := range tasks {
			assigned := "none"
			if t.AssignedAgent != nil && *t.AssignedAgent != "" {
				assigned = *t.AssignedAgent
			}
			scopeStr := ""
			if len(t.Scope) > 0 {
				scopeStr = fmt.Sprintf(" [scope: %s]", strings.Join(t.Scope, ", "))
			}
			fmt.Printf("- %s [%s] %s (agent: %s)%s\n", t.ID, t.Status, t.Title, assigned, scopeStr)
		}
		return nil
	},
}

var taskClaimCmd = &cobra.Command{
	Use:   "claim <task_id>",
	Short: "Claim a task for an agent and reserve its declared scope",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		if taskClaimAgent == "" {
			ui.Error("Agent is required. Use --agent <name>.")
			return errors.New("missing agent flag")
		}

		taskID := args[0]
		ok, msg, _, err := task.ClaimTask(root, taskID, taskClaimAgent)
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

var taskCompleteCmd = &cobra.Command{
	Use:   "complete <task_id>",
	Short: "Mark a task as completed and release its locks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		taskID := args[0]
		var ag *string
		if taskCompAgent != "" {
			ag = &taskCompAgent
		}

		ok, msg, _, err := task.CompleteTask(root, taskID, ag, true)
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

var taskReleaseCmd = &cobra.Command{
	Use:   "release <task_id>",
	Short: "Release a claimed task back to pending and release locks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		taskID := args[0]
		ok, msg, err := task.ReleaseTask(root, taskID)
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

var taskDeleteCmd = &cobra.Command{
	Use:   "delete <task_id>",
	Short: "Delete a task from the board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		taskID := args[0]
		ok, err := task.DeleteTask(root, taskID)
		if err != nil {
			return err
		}

		if ok {
			ui.Successf("Deleted task '%s'.", taskID)
			return nil
		}
		ui.Warningf("Task '%s' was not found.", taskID)
		return nil
	},
}

func init() {
	taskAddCmd.Flags().StringVarP(&taskDesc, "desc", "d", "", "Detailed task description")
	taskAddCmd.Flags().StringSliceVarP(&taskScope, "scope", "s", nil, "File path or glob pattern reserved for this task")

	taskListCmd.Flags().StringVar(&taskStatusFilter, "status", "", "Filter by status (pending, in_progress, review, done, cancelled)")
	taskListCmd.Flags().StringVar(&taskAgentFilter, "agent", "", "Filter by assigned agent")

	taskClaimCmd.Flags().StringVarP(&taskClaimAgent, "agent", "a", "", "Agent claiming the task (required)")
	taskCompleteCmd.Flags().StringVarP(&taskCompAgent, "agent", "a", "", "Agent completing the task")

	taskCmd.AddCommand(taskAddCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskClaimCmd)
	taskCmd.AddCommand(taskCompleteCmd)
	taskCmd.AddCommand(taskReleaseCmd)
	taskCmd.AddCommand(taskDeleteCmd)

	rootCmd.AddCommand(taskCmd)
}
