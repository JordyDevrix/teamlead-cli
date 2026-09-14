package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	regToolName     string
	regRoleName     string
	unregKeepWorktree bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage registered agents and collaborators",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents and their statuses",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		agents, err := agent.ListAgents(root)
		if err != nil {
			return err
		}

		if len(agents) == 0 {
			ui.Info("No agents registered.")
			return nil
		}

		for _, a := range agents {
			tool := "AI"
			if a.Tool != nil && *a.Tool != "" {
				tool = *a.Tool
			}
			taskID := "none"
			if a.CurrentTaskID != nil && *a.CurrentTaskID != "" {
				taskID = *a.CurrentTaskID
			}
			fmt.Printf("- %s (%s) | status: %s | task: %s\n", a.Name, tool, a.Status, taskID)
		}
		return nil
	},
}

var agentRegisterCmd = &cobra.Command{
	Use:   "register <name>",
	Short: "Register an agent manually",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		name := args[0]
		var tool *string
		if regToolName != "" {
			tool = &regToolName
		}
		var role *string
		if regRoleName != "" {
			role = &regRoleName
		}

		rec, err := agent.RegisterAgent(root, name, tool, role, nil, nil, nil)
		if err != nil {
			return err
		}

		ui.Successf("Registered agent '%s'.", rec.Name)
		return nil
	},
}

var agentUnregisterCmd = &cobra.Command{
	Use:   "unregister <name>",
	Short: "Unregister an agent and release locks",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not in a teamlead repo.")
			return errors.New("uninitialized repo")
		}

		name := args[0]
		ok, err := agent.UnregisterAgent(root, name, !unregKeepWorktree)
		if err != nil {
			return err
		}

		if ok {
			ui.Successf("Unregistered agent '%s'.", name)
		} else {
			ui.Warningf("Agent '%s' was not found.", name)
		}
		return nil
	},
}

func init() {
	agentRegisterCmd.Flags().StringVar(&regToolName, "tool", "", "Tool or model identifier (e.g. claude, aider)")
	agentRegisterCmd.Flags().StringVar(&regRoleName, "role", "", "Agent role or specialty")

	agentUnregisterCmd.Flags().BoolVar(&unregKeepWorktree, "keep-worktree", false, "Do not delete agent's worktree")

	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentRegisterCmd)
	agentCmd.AddCommand(agentUnregisterCmd)

	rootCmd.AddCommand(agentCmd)
}
