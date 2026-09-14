package task

import (
	"os"
	"strings"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
)

func TestTaskCreationAndListing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-task-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)

	t1, err := CreateTask(tmpDir, "Build auth", nil, []string{"src/auth/**"})
	if err != nil {
		t.Fatal(err)
	}
	t2, err := CreateTask(tmpDir, "Build UI", nil, []string{"frontend/**"})
	if err != nil {
		t.Fatal(err)
	}

	if t1.ID != "T-1" {
		t.Errorf("expected ID 'T-1', got '%s'", t1.ID)
	}
	if t2.ID != "T-2" {
		t.Errorf("expected ID 'T-2', got '%s'", t2.ID)
	}
	if t1.Status != StatusPending {
		t.Errorf("expected status 'pending', got '%s'", t1.Status)
	}

	tasks, err := ListTasks(tmpDir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	found, err := GetTask(tmpDir, "T-1")
	if err != nil || found == nil {
		t.Fatalf("expected task T-1 found, got %v", found)
	}
	if found.Title != "Build auth" {
		t.Errorf("expected 'Build auth', got '%s'", found.Title)
	}
}

func TestTaskClaimWithScopeLocking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-task-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)

	_, _ = CreateTask(tmpDir, "Build auth", nil, []string{"src/auth/**"})
	_, _ = CreateTask(tmpDir, "Fix login bug", nil, []string{"src/auth/login.py"})

	// Agent 1 claims T-1
	ok, _, task1, err := ClaimTask(tmpDir, "T-1", "agent-claude")
	if err != nil || !ok {
		t.Fatalf("expected ClaimTask T-1 to succeed, got %v", ok)
	}
	if task1.Status != StatusInProgress {
		t.Errorf("expected status in_progress, got %s", task1.Status)
	}
	if task1.AssignedAgent == nil || *task1.AssignedAgent != "agent-claude" {
		t.Errorf("expected assigned agent agent-claude, got %v", task1.AssignedAgent)
	}

	// Locks should now be held on src/auth/**
	locks, err := lock.GetActiveLocks(tmpDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 1 || locks[0].Path != "src/auth/**" || locks[0].Agent != "agent-claude" {
		t.Errorf("expected lock on src/auth/** held by agent-claude, got %v", locks)
	}

	// Agent 2 tries to claim T-2 which scopes src/auth/login.py -> should fail due to lock conflict
	ok2, msg2, _, err2 := ClaimTask(tmpDir, "T-2", "agent-aider")
	if err2 != nil {
		t.Fatal(err2)
	}
	if ok2 {
		t.Error("expected claim T-2 to fail due to lock conflict")
	}
	if !strings.Contains(msg2, "conflicts with active locks") {
		t.Errorf("expected conflict message, got: %s", msg2)
	}

	// Agent 1 completes T-1
	ok3, _, compTask, err3 := CompleteTask(tmpDir, "T-1", nil, true)
	if err3 != nil || !ok3 {
		t.Fatalf("expected complete T-1 to succeed, got %v", ok3)
	}
	if compTask.Status != StatusDone {
		t.Errorf("expected status 'done', got %s", compTask.Status)
	}

	// Locks should now be released
	locksAfter, _ := lock.GetActiveLocks(tmpDir, true)
	if len(locksAfter) != 0 {
		t.Errorf("expected 0 locks after completion, got %d", len(locksAfter))
	}

	// Now Agent 2 can claim T-2!
	ok4, _, task2Now, err4 := ClaimTask(tmpDir, "T-2", "agent-aider")
	if err4 != nil || !ok4 {
		t.Fatalf("expected Agent 2 to claim T-2 now, got %v (err: %v)", ok4, err4)
	}
	if task2Now.Status != StatusInProgress {
		t.Errorf("expected status 'in_progress', got %s", task2Now.Status)
	}
}

func TestTaskRelease(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-task-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_ = config.SaveConfig(config.DefaultConfig(), tmpDir)

	_, _ = CreateTask(tmpDir, "Refactor", nil, []string{"src/core/*"})
	_, _, _, _ = ClaimTask(tmpDir, "T-1", "agent-gemini")

	locks, _ := lock.GetActiveLocks(tmpDir, true)
	if len(locks) != 1 {
		t.Fatalf("expected 1 lock, got %d", len(locks))
	}

	// Release task back to pool
	ok, _, err := ReleaseTask(tmpDir, "T-1")
	if err != nil || !ok {
		t.Fatalf("expected ReleaseTask to succeed, got %v", ok)
	}

	t1, _ := GetTask(tmpDir, "T-1")
	if t1.Status != StatusPending {
		t.Errorf("expected status 'pending', got '%s'", t1.Status)
	}
	if t1.AssignedAgent != nil {
		t.Errorf("expected assigned agent nil, got %v", t1.AssignedAgent)
	}

	locksAfter, _ := lock.GetActiveLocks(tmpDir, true)
	if len(locksAfter) != 0 {
		t.Errorf("expected 0 locks after release, got %d", len(locksAfter))
	}
}
