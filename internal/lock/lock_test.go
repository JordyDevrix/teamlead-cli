package lock

import (
	"os"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
)

func TestPatternsOverlap(t *testing.T) {
	// Exact match
	if !PatternsOverlap("src/auth.py", "src/auth.py") {
		t.Error("expected src/auth.py to match src/auth.py")
	}
	if !PatternsOverlap("src/auth.py", "./src/auth.py") {
		t.Error("expected src/auth.py to match ./src/auth.py")
	}

	// Directory / glob overlap
	if !PatternsOverlap("src/auth/**", "src/auth/service.py") {
		t.Error("expected src/auth/** to match src/auth/service.py")
	}
	if !PatternsOverlap("src/auth/*", "src/auth/login.py") {
		t.Error("expected src/auth/* to match src/auth/login.py")
	}
	if !PatternsOverlap("src/auth", "src/auth/service.py") {
		t.Error("expected src/auth to match src/auth/service.py")
	}
	if !PatternsOverlap("src/auth/service.py", "src/auth") {
		t.Error("expected src/auth/service.py to match src/auth")
	}

	// Universal wildcards
	if !PatternsOverlap("*", "anything.py") {
		t.Error("expected * to match anything.py")
	}
	if !PatternsOverlap("**", "src/nested/deep/file.py") {
		t.Error("expected ** to match src/nested/deep/file.py")
	}

	// Non-overlapping
	if PatternsOverlap("src/auth/**", "src/database/**") {
		t.Error("expected src/auth/** and src/database/** not to overlap")
	}
	if PatternsOverlap("src/auth/service.py", "src/auth/model.py") {
		t.Error("expected src/auth/service.py and src/auth/model.py not to overlap")
	}
	if PatternsOverlap("backend/*", "frontend/*") {
		t.Error("expected backend/* and frontend/* not to overlap")
	}
}

func TestLockAcquireAndConflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-lock-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)

	taskID1 := "T-1"
	reason1 := "Refactoring authentication"
	ttl := 30

	// Agent A acquires lock on auth module
	ok, acquired, conflicts, err := AcquireLocks(tmpDir, []string{"src/auth/**"}, "agent-a", &taskID1, &reason1, &ttl)
	if err != nil || !ok {
		t.Fatalf("expected AcquireLocks to succeed, got %v (err: %v)", ok, err)
	}
	if len(acquired) != 1 {
		t.Errorf("expected 1 acquired lock, got %d", len(acquired))
	}
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts, got %d", len(conflicts))
	}

	// Agent B tries to acquire lock on a file inside auth module -> should fail
	taskID2 := "T-2"
	ok2, acquired2, conflicts2, err2 := AcquireLocks(tmpDir, []string{"src/auth/login.py"}, "agent-b", &taskID2, nil, nil)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if ok2 {
		t.Error("expected lock acquisition to fail due to conflict")
	}
	if len(acquired2) != 0 {
		t.Errorf("expected 0 acquired locks, got %d", len(acquired2))
	}
	if len(conflicts2) != 1 || conflicts2[0].Agent != "agent-a" {
		t.Errorf("expected 1 conflict with agent-a, got %v", conflicts2)
	}

	// Agent B acquires lock on unrelated file -> should succeed
	ok3, acquired3, conflicts3, err3 := AcquireLocks(tmpDir, []string{"src/database/models.py"}, "agent-b", &taskID2, nil, nil)
	if err3 != nil || !ok3 {
		t.Fatalf("expected acquire to succeed, got %v (err: %v)", ok3, err3)
	}
	if len(acquired3) != 1 || len(conflicts3) != 0 {
		t.Errorf("expected 1 acquired lock and 0 conflicts, got %d and %d", len(acquired3), len(conflicts3))
	}

	// Check conflicts helper
	confs, err := CheckConflicts(tmpDir, []string{"src/auth/token.py"}, "agent-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(confs) != 1 || confs[0].Agent != "agent-a" {
		t.Errorf("expected 1 conflict with agent-a, got %v", confs)
	}

	// Agent A itself should not conflict with its own locks
	confsSelf, err := CheckConflicts(tmpDir, []string{"src/auth/token.py"}, "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(confsSelf) != 0 {
		t.Errorf("expected 0 conflicts for agent-a checking self, got %d", len(confsSelf))
	}
}

func TestLockReleaseAndRefresh(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-lock-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)

	_, _, _, _ = AcquireLocks(tmpDir, []string{"src/a.py", "src/b.py"}, "agent-1", nil, nil, nil)
	_, _, _, _ = AcquireLocks(tmpDir, []string{"src/c.py"}, "agent-2", nil, nil, nil)

	locks, err := GetActiveLocks(tmpDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 3 {
		t.Fatalf("expected 3 locks, got %d", len(locks))
	}

	// Release single lock
	released, err := ReleaseLocks(tmpDir, []string{"src/a.py"}, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0] != "src/a.py" {
		t.Errorf("expected ['src/a.py'] released, got %v", released)
	}

	locks, _ = GetActiveLocks(tmpDir, true)
	if len(locks) != 2 {
		t.Errorf("expected 2 remaining locks, got %d", len(locks))
	}

	// Refresh locks
	ttl45 := 45
	refreshed, err := RefreshAgentLocks(tmpDir, "agent-1", &ttl45)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != 1 {
		t.Errorf("expected 1 refreshed lock, got %d", refreshed)
	}

	// Release all for agent
	count, err := ReleaseAllForAgent(tmpDir, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 released lock, got %d", count)
	}

	remaining, _ := GetActiveLocks(tmpDir, true)
	if len(remaining) != 1 || remaining[0].Agent != "agent-2" {
		t.Errorf("expected 1 lock for agent-2 remaining, got %v", remaining)
	}
}
