package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/context"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	runAgentName   string
	runTaskID      string
	runToolName    string
	runLockPaths   []string
	runNoWorktree  bool
	runBranchName  string
)

var runCmd = &cobra.Command{
	Use:   "run --agent <NAME> [flags] -- <COMMAND...>",
	Short: "Run an agent command inside an isolated worktree with file locking and coordination",
	Long: `Run an agent command inside an isolated worktree with file locking and coordination.

Examples:
  teamlead run --agent claude --task T-1 -- claude
  teamlead run --agent aider --lock "src/auth/**" -- aider
  teamlead run --agent custom-bot -- python3 my_script.py`,
	DisableFlagParsing: false,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := config.FindRepoRoot("")
		if err != nil || !config.IsInitialized(root) {
			ui.Error("Not inside an initialized teamlead repository. Run 'teamlead init' first.")
			return errors.New("uninitialized repository")
		}

		if runAgentName == "" {
			ui.Error("Agent name is required. Use --agent <name>.")
			return errors.New("missing agent name")
		}

		// Look for command arguments after --
		cArgs := cmd.Flags().Args()
		if len(cArgs) == 0 {
			ui.Error("No command specified to run. Usage: teamlead run --agent <NAME> -- <COMMAND...>")
			return errors.New("missing command")
		}

		cfg, err := config.LoadConfig(root)
		if err != nil {
			return err
		}

		// 1. Claim task if specified
		if runTaskID != "" {
			ok, msg, _, err := task.ClaimTask(root, runTaskID, runAgentName)
			if err != nil {
				return err
			}
			if !ok {
				ui.Error(msg)
				return errors.New(msg)
			}
			ui.Success(msg)
		}

		// 2. Acquire additional locks
		if len(runLockPaths) > 0 {
			var tID *string
			if runTaskID != "" {
				tID = &runTaskID
			}
			reason := fmt.Sprintf("Running agent %s", runAgentName)
			ok, acquired, conflicts, err := lock.AcquireLocks(root, runLockPaths, runAgentName, tID, &reason, nil)
			if err != nil {
				return err
			}
			if !ok {
				var confStrs []string
				for _, c := range conflicts {
					confStrs = append(confStrs, fmt.Sprintf("'%s' (locked by %s)", c.Path, c.Agent))
				}
				ui.Errorf("Cannot acquire locks: conflict with existing leases: %s", strings.Join(confStrs, ", "))
				return errors.New("lock conflict")
			}
			var acqPaths []string
			for _, l := range acquired {
				acqPaths = append(acqPaths, l.Path)
			}
			ui.Successf("Acquired locks on: %s", strings.Join(acqPaths, ", "))
		}

		// 3. Worktree setup
		executionCwd := root
		var worktreePath *string
		branchName := runBranchName
		if branchName == "" {
			sub := "workspace"
			if runTaskID != "" {
				sub = runTaskID
			}
			branchName = fmt.Sprintf("teamlead/%s/%s", runAgentName, sub)
		}

		if !runNoWorktree {
			wtDir := filepath.Join(root, cfg.WorktreeDir, runAgentName)
			worktreePath = &wtDir

			if fi, err := os.Stat(wtDir); err != nil || !fi.IsDir() {
				ui.Infof("Creating isolated worktree for agent %s at %s...", runAgentName, wtDir)
				if err := git.CreateWorktree(root, wtDir, branchName, cfg.BaseBranch); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			} else {
				ui.Infof("Using existing worktree at %s", wtDir)
				if cfg.AutoSync {
					syncOk, syncMsg, err := git.SyncWorktreeBranch(wtDir, cfg.BaseBranch, "rebase")
					if err != nil || !syncOk {
						ui.Warningf("Could not auto-sync worktree: %s", syncMsg)
					}
				}
			}

			// Inject COLLABORATION.md
			_, _ = context.InjectContextIntoWorktree(root, wtDir, runAgentName)
			executionCwd = wtDir
		}

		// 4. Register agent
		tool := runToolName
		if tool == "" && len(cArgs) > 0 {
			tool = filepath.Base(cArgs[0])
		}
		var tID *string
		if runTaskID != "" {
			tID = &runTaskID
		}
		curBranch := branchName
		if runNoWorktree {
			curBranch = git.GetCurrentBranch(root)
		}

		_, err = agent.RegisterAgent(root, runAgentName, &tool, nil, worktreePath, &curBranch, tID)
		if err != nil {
			return err
		}

		// 5. Heartbeat background thread
		stopHeartbeat := make(chan struct{})
		myPID := os.Getpid()

		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ticker.C:
					_, _ = agent.HeartbeatAgent(root, runAgentName, &myPID, tID, &curBranch, agent.StatusActive)
				}
			}
		}()

		// 6. Execute child process
		ui.Infof("Executing: %s in %s", strings.Join(cArgs, " "), executionCwd)

		childCmd := exec.Command(cArgs[0], cArgs[1:]...)
		childCmd.Dir = executionCwd
		childCmd.Stdin = os.Stdin
		childCmd.Stdout = os.Stdout
		childCmd.Stderr = os.Stderr

		// Handle signals and forward to child
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		exitCode := 0
		if err := childCmd.Start(); err != nil {
			close(stopHeartbeat)
			_, _ = agent.HeartbeatAgent(root, runAgentName, nil, tID, &curBranch, agent.StatusIdle)
			ui.Errorf("Failed to start command '%s': %v", cArgs[0], err)
			return err
		}

		// Forward interrupt
		go func() {
			for sig := range sigChan {
				if childCmd.Process != nil {
					_ = childCmd.Process.Signal(sig)
				}
			}
		}()

		runErr := childCmd.Wait()
		close(stopHeartbeat)
		signal.Stop(sigChan)
		close(sigChan)

		_, _ = agent.HeartbeatAgent(root, runAgentName, nil, tID, &curBranch, agent.StatusIdle)

		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}

		// 7. Post-execution summary
		if worktreePath != nil {
			st, err := git.GetWorktreeStatus(*worktreePath)
			if err == nil && st.Exists {
				if st.IsDirty {
					ui.Warningf("Worktree has %d uncommitted file(s). Commit them inside %s before merging.", len(st.AllDirtyFiles), *worktreePath)
				} else {
					ui.Successf("Agent finished. Changes are committed to branch '%s'.", branchName)
					ui.Infof("To integrate this work into '%s', run:", cfg.BaseBranch)
					fmt.Printf("  teamlead merge %s\n\n", runAgentName)
				}
			}
		}

		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}

func init() {
	runCmd.Flags().StringVarP(&runAgentName, "agent", "a", "", "Agent identifier (e.g. claude, aider, gemini)")
	runCmd.Flags().StringVarP(&runTaskID, "task", "t", "", "Task ID to claim and scope (e.g. T-1)")
	runCmd.Flags().StringVar(&runToolName, "tool", "", "Tool or model name")
	runCmd.Flags().StringSliceVarP(&runLockPaths, "lock", "l", nil, "Additional file paths or globs to lock")
	runCmd.Flags().BoolVar(&runNoWorktree, "no-worktree", false, "Run directly in repository root instead of an isolated worktree")
	runCmd.Flags().StringVar(&runBranchName, "branch", "", "Custom feature branch name for the worktree")

	rootCmd.AddCommand(runCmd)
}
