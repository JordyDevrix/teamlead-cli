package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
)

func createTestGitRepo(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tl-git-test-*")
	if err != nil {
		t.Fatal(err)
	}

	_, _ = RunGit(tmpDir, "init", "-b", "main")
	_, _ = RunGit(tmpDir, "config", "user.name", "Test Agent")
	_, _ = RunGit(tmpDir, "config", "user.email", "agent@teamlead.ai")
	if err := EnsureInitialCommit(tmpDir); err != nil {
		t.Fatal(err)
	}
	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)
	return tmpDir
}

func TestWorktreeLifecycle(t *testing.T) {
	repoDir := createTestGitRepo(t)
	defer os.RemoveAll(repoDir)

	wtDir := filepath.Join(repoDir, ".teamlead", "worktrees", "agent-claude")
	branchName := "teamlead/claude/task-1"

	// Create worktree
	if err := CreateWorktree(repoDir, wtDir, branchName, "main"); err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	if _, err := os.Stat(wtDir); err != nil {
		t.Fatalf("worktree directory does not exist: %v", err)
	}

	// Verify worktrees listed
	wts, err := ListWorktrees(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) < 2 {
		t.Errorf("expected at least 2 worktrees, got %d", len(wts))
	}

	// Check status (clean initially)
	status, err := GetWorktreeStatus(wtDir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Exists {
		t.Error("expected status.Exists to be true")
	}
	if status.IsDirty {
		t.Error("expected status.IsDirty to be false")
	}
	if status.Branch != branchName {
		t.Errorf("expected branch '%s', got '%s'", branchName, status.Branch)
	}

	// Create uncommitted file
	testFile := filepath.Join(wtDir, "service.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	statusDirty, _ := GetWorktreeStatus(wtDir)
	if !statusDirty.IsDirty {
		t.Error("expected worktree to be dirty")
	}

	// Commit file
	_, _ = RunGit(wtDir, "add", "service.go")
	_, _ = RunGit(wtDir, "commit", "-m", "feat: add service.go")

	statusClean, _ := GetWorktreeStatus(wtDir)
	if statusClean.IsDirty {
		t.Error("expected worktree to be clean after commit")
	}

	// Remove worktree
	if err := RemoveWorktree(repoDir, wtDir, true); err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory removed, but it still exists")
	}
}
