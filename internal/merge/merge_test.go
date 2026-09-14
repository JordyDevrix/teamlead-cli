package merge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
)

func setupMergeReadyRepo(t *testing.T) (string, string, *task.Task) {
	t.Helper()
	repoDir, err := os.MkdirTemp("", "tl-merge-test-*")
	if err != nil {
		t.Fatal(err)
	}

	_, _ = git.RunGit(repoDir, "init", "-b", "main")
	_, _ = git.RunGit(repoDir, "config", "user.name", "Test Agent")
	_, _ = git.RunGit(repoDir, "config", "user.email", "agent@teamlead.ai")
	if err := git.EnsureInitialCommit(repoDir); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.BaseBranch = "main"
	cfg.MergeStrategy = "squash"
	_ = config.SaveConfig(cfg, repoDir)

	taskItem, err := task.CreateTask(repoDir, "Add feature X", nil, []string{"feature_x.go"})
	if err != nil {
		t.Fatal(err)
	}

	wtPath := filepath.Join(repoDir, ".teamlead", "worktrees", "agent-coder")
	if err := git.CreateWorktree(repoDir, wtPath, "feature-x-branch", "main"); err != nil {
		t.Fatal(err)
	}

	branchName := "feature-x-branch"
	_, _ = agent.RegisterAgent(repoDir, "agent-coder", nil, nil, &wtPath, &branchName, &taskItem.ID)
	_, _, _, _ = lock.AcquireLocks(repoDir, []string{"feature_x.go"}, "agent-coder", &taskItem.ID, nil, nil)

	return repoDir, wtPath, taskItem
}

func TestMergeValidationCatchesDirtyWorktree(t *testing.T) {
	repoDir, wtPath, _ := setupMergeReadyRepo(t)
	defer os.RemoveAll(repoDir)

	// Leave uncommitted file
	_ = os.WriteFile(filepath.Join(wtPath, "feature_x.go"), []byte("package main\n"), 0644)

	ok, msg, err := ValidateAgentForMerge(repoRoot(repoDir), "agent-coder", true)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected validation to fail for dirty worktree")
	}
	if !strings.Contains(msg, "uncommitted changes") {
		t.Errorf("expected dirty changes in msg, got: %s", msg)
	}
}

func TestSuccessfulMergeAndCleanup(t *testing.T) {
	repoDir, wtPath, taskItem := setupMergeReadyRepo(t)
	defer os.RemoveAll(repoDir)

	// Commit file inside worktree
	_ = os.WriteFile(filepath.Join(wtPath, "feature_x.go"), []byte("package main\nfunc Feature() bool { return true }\n"), 0644)
	_, _ = git.RunGit(wtPath, "add", "feature_x.go")
	_, _ = git.RunGit(wtPath, "commit", "-m", "feat: implement feature x")

	// Validate before merge
	ok, _, err := ValidateAgentForMerge(repoDir, "agent-coder", true)
	if err != nil || !ok {
		t.Fatalf("expected validation success, got %v (err: %v)", ok, err)
	}

	// Perform merge
	strategy := "squash"
	okMerge, mergeMsg, err := MergeAgentWork(repoDir, "agent-coder", &strategy, true, true, true)
	if err != nil || !okMerge {
		t.Fatalf("expected merge to succeed, got %v (msg: %s, err: %v)", okMerge, mergeMsg, err)
	}

	// Verify file now in main repository
	mergedFile := filepath.Join(repoDir, "feature_x.go")
	data, err := os.ReadFile(mergedFile)
	if err != nil {
		t.Fatalf("expected merged file %s to exist: %v", mergedFile, err)
	}
	if !strings.Contains(string(data), "func Feature()") {
		t.Errorf("expected func Feature() in file, got %s", string(data))
	}

	// Verify locks released
	locks, _ := lock.GetActiveLocks(repoDir, true)
	if len(locks) != 0 {
		t.Errorf("expected 0 locks remaining, got %d", len(locks))
	}

	// Verify task completed
	tUpdated, _ := task.GetTask(repoDir, taskItem.ID)
	if tUpdated == nil || tUpdated.Status != task.StatusDone {
		t.Errorf("expected task marked done, got %v", tUpdated)
	}

	// Verify agent unregistered
	ag, _ := agent.GetAgent(repoDir, "agent-coder")
	if ag != nil {
		t.Errorf("expected agent unregistered, got %v", ag)
	}

	// Verify worktree cleaned up
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory deleted, but still exists")
	}
}

func repoRoot(p string) string {
	return p
}
