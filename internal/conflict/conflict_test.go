package conflict

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
)

func setupRepoWithTwoAgents(t *testing.T) (string, string, string) {
	t.Helper()
	repoDir, err := os.MkdirTemp("", "tl-conflict-test-*")
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
	_ = config.SaveConfig(cfg, repoDir)

	wtA := filepath.Join(repoDir, ".teamlead", "worktrees", "agent-a")
	wtB := filepath.Join(repoDir, ".teamlead", "worktrees", "agent-b")

	if err := git.CreateWorktree(repoDir, wtA, "branch-a", "main"); err != nil {
		t.Fatal(err)
	}
	if err := git.CreateWorktree(repoDir, wtB, "branch-b", "main"); err != nil {
		t.Fatal(err)
	}

	branchA := "branch-a"
	branchB := "branch-b"
	_, _ = agent.RegisterAgent(repoDir, "agent-a", nil, nil, &wtA, &branchA, nil)
	_, _ = agent.RegisterAgent(repoDir, "agent-b", nil, nil, &wtB, &branchB, nil)

	return repoDir, wtA, wtB
}

func TestConflictDetectionClean(t *testing.T) {
	repoDir, _, _ := setupRepoWithTwoAgents(t)
	defer os.RemoveAll(repoDir)

	report, err := DetectConflicts(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if report.HasBlockingConflicts {
		t.Error("expected no blocking conflicts in clean state")
	}
	if len(report.DirectFileConflicts) != 0 {
		t.Errorf("expected 0 direct file conflicts, got %d", len(report.DirectFileConflicts))
	}
}

func TestConflictDetectionDirectCollision(t *testing.T) {
	repoDir, wtA, wtB := setupRepoWithTwoAgents(t)
	defer os.RemoveAll(repoDir)

	// Agent A modifies auth.go
	_ = os.WriteFile(filepath.Join(wtA, "auth.go"), []byte("// agent a modification\n"), 0644)
	_, _ = git.RunGit(wtA, "add", "auth.go")
	_, _ = git.RunGit(wtA, "commit", "-m", "agent a auth edit")

	// Agent B ALSO modifies auth.go
	_ = os.WriteFile(filepath.Join(wtB, "auth.go"), []byte("// agent b modification\n"), 0644)
	_, _ = git.RunGit(wtB, "add", "auth.go")
	_, _ = git.RunGit(wtB, "commit", "-m", "agent b auth edit")

	report, err := DetectConflicts(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasBlockingConflicts {
		t.Error("expected blocking conflicts to be true")
	}
	if len(report.DirectFileConflicts) != 1 {
		t.Fatalf("expected 1 direct collision, got %d", len(report.DirectFileConflicts))
	}

	collision := report.DirectFileConflicts[0]
	if collision.File != "auth.go" {
		t.Errorf("expected collision on auth.go, got %s", collision.File)
	}
}

func TestLockViolationDetection(t *testing.T) {
	repoDir, _, wtB := setupRepoWithTwoAgents(t)
	defer os.RemoveAll(repoDir)

	// Agent A locks payments.go
	_, _, _, err := lock.AcquireLocks(repoDir, []string{"payments.go"}, "agent-a", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Agent B modifies payments.go in its worktree without acquiring lock
	_ = os.WriteFile(filepath.Join(wtB, "payments.go"), []byte("// unauthorized edit\n"), 0644)

	report, err := DetectConflicts(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasBlockingConflicts {
		t.Error("expected blocking conflict for lock violation")
	}
	if len(report.LockViolations) != 1 {
		t.Fatalf("expected 1 lock violation, got %d", len(report.LockViolations))
	}

	viol := report.LockViolations[0]
	if viol.File != "payments.go" {
		t.Errorf("expected payments.go violated, got %s", viol.File)
	}
	if viol.ModifyingAgent != "agent-b" {
		t.Errorf("expected modifying agent agent-b, got %s", viol.ModifyingAgent)
	}
	if viol.LockingAgent != "agent-a" {
		t.Errorf("expected locking agent agent-a, got %s", viol.LockingAgent)
	}
}
