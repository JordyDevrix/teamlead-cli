package merge

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/context"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/storage"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
)

// ValidateAgentForMerge verifies worktree is clean, committed, and passes tests.
func ValidateAgentForMerge(repoRoot, agentName string, runTests bool) (bool, string, error) {
	ag, err := agent.GetAgent(repoRoot, agentName)
	if err != nil {
		return false, "", err
	}
	if ag == nil {
		return false, fmt.Sprintf("Agent '%s' not found.", agentName), nil
	}

	if ag.WorktreePath == nil || *ag.WorktreePath == "" {
		return false, fmt.Sprintf("Agent '%s' does not have an assigned worktree.", agentName), nil
	}

	wtPath := *ag.WorktreePath
	wtStatus, err := git.GetWorktreeStatus(wtPath)
	if err != nil {
		return false, "", err
	}
	if !wtStatus.Exists {
		return false, fmt.Sprintf("Worktree directory %s does not exist.", wtPath), nil
	}

	// 1. Check uncommitted changes
	if wtStatus.IsDirty {
		dirtyFiles := strings.Join(wtStatus.AllDirtyFiles, ", ")
		return false, fmt.Sprintf("Worktree has uncommitted changes: %s. Please commit them before merging.", dirtyFiles), nil
	}

	// 2. Check branch
	branch := wtStatus.Branch
	if branch == "" && ag.CurrentBranch != nil {
		branch = *ag.CurrentBranch
	}
	if branch == "" {
		return false, fmt.Sprintf("Could not determine git branch for agent '%s'.", agentName), nil
	}

	// 3. Optional test validation inside worktree
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return false, "", err
	}

	if runTests && cfg.TestCommand != nil && strings.TrimSpace(*cfg.TestCommand) != "" {
		cmdStr := strings.TrimSpace(*cfg.TestCommand)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", cmdStr)
		} else {
			cmd = exec.Command("sh", "-c", cmdStr)
		}
		cmd.Dir = wtPath

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf

		if err := cmd.Run(); err != nil {
			errOutput := strings.TrimSpace(outBuf.String())
			return false, fmt.Sprintf("Pre-merge validation test failed ('%s'):\n%s", cmdStr, errOutput), nil
		}
	}

	return true, "Validation successful.", nil
}

// MergeAgentWork validates, merges into base branch, releases locks, and cleans up.
func MergeAgentWork(repoRoot, agentName string, strategy *string, deleteBranchAfter, cleanupWorktree, runTests bool) (bool, string, error) {
	ok, valMsg, err := ValidateAgentForMerge(repoRoot, agentName, runTests)
	if err != nil {
		return false, "", err
	}
	if !ok {
		return false, valMsg, nil
	}

	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return false, "", err
	}

	ag, err := agent.GetAgent(repoRoot, agentName)
	if err != nil {
		return false, "", err
	}
	if ag == nil {
		return false, fmt.Sprintf("Agent '%s' not found.", agentName), nil
	}

	wtPath := ""
	if ag.WorktreePath != nil {
		wtPath = *ag.WorktreePath
	}

	sourceBranch := ""
	if ag.CurrentBranch != nil {
		sourceBranch = *ag.CurrentBranch
	}
	if wtPath != "" {
		st, _ := git.GetWorktreeStatus(wtPath)
		if st != nil && st.Branch != "" {
			sourceBranch = st.Branch
		}
	}

	if sourceBranch == "" {
		return false, fmt.Sprintf("Cannot determine branch for agent '%s'.", agentName), nil
	}

	baseBranch := cfg.BaseBranch
	strat := cfg.MergeStrategy
	if strategy != nil && *strategy != "" {
		strat = *strategy
	}

	commitMsg := fmt.Sprintf("feat(teamlead): integrate changes from agent %s (%s)", agentName, sourceBranch)
	if ag.CurrentTaskID != nil && *ag.CurrentTaskID != "" {
		t, _ := task.GetTask(repoRoot, *ag.CurrentTaskID)
		if t != nil {
			commitMsg = fmt.Sprintf("feat(%s): %s (by %s)", t.ID, t.Title, agentName)
		}
	}

	// Execute git merge on main repo
	mergedOk, mergeMsg, err := git.MergeBranchIntoBase(repoRoot, sourceBranch, baseBranch, strat, commitMsg)
	if err != nil {
		return false, "", err
	}
	if !mergedOk {
		return false, fmt.Sprintf("Merge failed: %s", mergeMsg), nil
	}

	// Release all locks held by agent
	_, _ = lock.ReleaseAllForAgent(repoRoot, agentName)

	// Mark task completed if any
	if ag.CurrentTaskID != nil && *ag.CurrentTaskID != "" {
		_, _, _, _ = task.CompleteTask(repoRoot, *ag.CurrentTaskID, &agentName, true)
	}

	// Remove worktree
	if cleanupWorktree && wtPath != "" {
		_ = git.RemoveWorktree(repoRoot, wtPath, true)
	}

	// Delete branch
	if deleteBranchAfter {
		_ = git.DeleteBranch(repoRoot, sourceBranch, true)
	}

	// Unregister agent
	_, _ = agent.UnregisterAgent(repoRoot, agentName, false)

	storage.LogEvent(repoRoot, "MERGE_COMPLETED", &agentName, map[string]interface{}{
		"source_branch": sourceBranch,
		"base_branch":   baseBranch,
		"strategy":      strat,
		"task_id":       ag.CurrentTaskID,
	})

	_, _ = context.UpdateRootAgentsMD(repoRoot)

	return true, fmt.Sprintf("Successfully integrated %s into %s (%s).", sourceBranch, baseBranch, strat), nil
}
