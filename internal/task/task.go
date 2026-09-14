package task

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/storage"
)

const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusReview     = "review"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

// Task represents a coordinated work unit.
type Task struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   *string  `json:"description"`
	Status        string   `json:"status"`
	AssignedAgent *string  `json:"assigned_agent"`
	Scope         []string `json:"scope"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// ListTasks returns all tasks, optionally filtering by status or assigned agent.
func ListTasks(repoRoot string, statusFilter, agentFilter *string) ([]Task, error) {
	var tasks []Task
	if err := storage.ReadJSONFile(repoRoot, config.TasksFile, &tasks); err != nil {
		return nil, err
	}

	var filtered []Task
	for _, t := range tasks {
		if statusFilter != nil && *statusFilter != "" && !strings.EqualFold(t.Status, *statusFilter) {
			continue
		}
		if agentFilter != nil && *agentFilter != "" {
			if t.AssignedAgent == nil || !strings.EqualFold(*t.AssignedAgent, *agentFilter) {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	return filtered, nil
}

// GetTask looks up a task by ID (case-insensitive).
func GetTask(repoRoot string, taskID string) (*Task, error) {
	tasks, err := ListTasks(repoRoot, nil, nil)
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		if strings.EqualFold(t.ID, taskID) {
			copy := t
			return &copy, nil
		}
	}
	return nil, nil
}

// CreateTask adds a new task with an auto-incremented ID (e.g. T-1, T-2).
func CreateTask(repoRoot, title string, description *string, scope []string) (*Task, error) {
	var created *Task
	cleanTitle := strings.TrimSpace(title)
	var cleanDesc *string
	if description != nil && strings.TrimSpace(*description) != "" {
		d := strings.TrimSpace(*description)
		cleanDesc = &d
	}

	var cleanScope []string
	for _, s := range scope {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			cleanScope = append(cleanScope, trimmed)
		}
	}

	err := storage.WithFileLock(repoRoot, func() error {
		existing, err := ListTasks(repoRoot, nil, nil)
		if err != nil {
			return err
		}

		maxNum := 0
		for _, t := range existing {
			if strings.HasPrefix(strings.ToUpper(t.ID), "T-") {
				numStr := t.ID[2:]
				if n, err := strconv.Atoi(numStr); err == nil && n > maxNum {
					maxNum = n
				}
			}
		}

		newID := fmt.Sprintf("T-%d", maxNum+1)
		nowStr := time.Now().UTC().Format(time.RFC3339)

		t := Task{
			ID:            newID,
			Title:         cleanTitle,
			Description:   cleanDesc,
			Status:        StatusPending,
			AssignedAgent: nil,
			Scope:         cleanScope,
			CreatedAt:     nowStr,
			UpdatedAt:     nowStr,
		}

		existing = append(existing, t)
		if err := storage.WriteJSONFile(repoRoot, config.TasksFile, existing); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "TASK_CREATED", nil, map[string]interface{}{
			"task_id": newID,
			"title":   cleanTitle,
			"scope":   cleanScope,
		})

		created = &t
		return nil
	})

	return created, err
}

// ClaimTask claims a task for an agent and acquires locks for its declared scopes.
func ClaimTask(repoRoot, taskID, agent string) (bool, string, *Task, error) {
	var claimed *Task
	var resultMsg string
	var success bool

	err := storage.WithFileLock(repoRoot, func() error {
		tasks, err := ListTasks(repoRoot, nil, nil)
		if err != nil {
			return err
		}

		idx := -1
		for i, t := range tasks {
			if strings.EqualFold(t.ID, taskID) {
				idx = i
				break
			}
		}

		if idx == -1 {
			resultMsg = fmt.Sprintf("Task '%s' not found", taskID)
			return nil
		}

		target := &tasks[idx]
		if target.Status == StatusInProgress && target.AssignedAgent != nil && !strings.EqualFold(*target.AssignedAgent, agent) {
			resultMsg = fmt.Sprintf("Task '%s' is already claimed by agent '%s'", taskID, *target.AssignedAgent)
			return nil
		}

		// If task has scope, acquire locks
		if len(target.Scope) > 0 {
			reason := fmt.Sprintf("Working on %s: %s", target.ID, target.Title)
			ok, _, conflicts, err := lock.AcquireLocks(repoRoot, target.Scope, agent, &target.ID, &reason, nil)
			if err != nil {
				return err
			}
			if !ok {
				var conflictStrs []string
				for _, c := range conflicts {
					conflictStrs = append(conflictStrs, fmt.Sprintf("'%s' locked by %s", c.Path, c.Agent))
				}
				resultMsg = fmt.Sprintf("Cannot claim task '%s': scope conflicts with active locks (%s)", taskID, strings.Join(conflictStrs, ", "))
				return nil
			}
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		target.Status = StatusInProgress
		target.AssignedAgent = &agent
		target.UpdatedAt = nowStr

		if err := storage.WriteJSONFile(repoRoot, config.TasksFile, tasks); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "TASK_CLAIMED", &agent, map[string]interface{}{
			"task_id": target.ID,
			"title":   target.Title,
			"scope":   target.Scope,
		})

		claimed = target
		success = true
		resultMsg = fmt.Sprintf("Task '%s' successfully claimed by agent '%s'", target.ID, agent)
		return nil
	})

	return success, resultMsg, claimed, err
}

// CompleteTask marks a task as done and releases its scope locks.
func CompleteTask(repoRoot, taskID string, agent *string, releaseScopeLocks bool) (bool, string, *Task, error) {
	var completed *Task
	var resultMsg string
	var success bool

	err := storage.WithFileLock(repoRoot, func() error {
		tasks, err := ListTasks(repoRoot, nil, nil)
		if err != nil {
			return err
		}

		idx := -1
		for i, t := range tasks {
			if strings.EqualFold(t.ID, taskID) {
				idx = i
				break
			}
		}

		if idx == -1 {
			resultMsg = fmt.Sprintf("Task '%s' not found", taskID)
			return nil
		}

		target := &tasks[idx]
		effectiveAgent := agent
		if effectiveAgent == nil || *effectiveAgent == "" {
			effectiveAgent = target.AssignedAgent
		}

		if releaseScopeLocks && effectiveAgent != nil && len(target.Scope) > 0 {
			_, _ = lock.ReleaseLocks(repoRoot, target.Scope, *effectiveAgent)
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		target.Status = StatusDone
		target.UpdatedAt = nowStr

		if err := storage.WriteJSONFile(repoRoot, config.TasksFile, tasks); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "TASK_COMPLETED", effectiveAgent, map[string]interface{}{
			"task_id": target.ID,
			"title":   target.Title,
		})

		completed = target
		success = true
		resultMsg = fmt.Sprintf("Task '%s' marked as done", target.ID)
		return nil
	})

	return success, resultMsg, completed, err
}

// ReleaseTask releases an in-progress task back to pending.
func ReleaseTask(repoRoot, taskID string) (bool, string, error) {
	var resultMsg string
	var success bool

	err := storage.WithFileLock(repoRoot, func() error {
		tasks, err := ListTasks(repoRoot, nil, nil)
		if err != nil {
			return err
		}

		idx := -1
		for i, t := range tasks {
			if strings.EqualFold(t.ID, taskID) {
				idx = i
				break
			}
		}

		if idx == -1 {
			resultMsg = fmt.Sprintf("Task '%s' not found", taskID)
			return nil
		}

		target := &tasks[idx]
		prevAgent := target.AssignedAgent
		if prevAgent != nil && len(target.Scope) > 0 {
			_, _ = lock.ReleaseLocks(repoRoot, target.Scope, *prevAgent)
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		target.Status = StatusPending
		target.AssignedAgent = nil
		target.UpdatedAt = nowStr

		if err := storage.WriteJSONFile(repoRoot, config.TasksFile, tasks); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "TASK_RELEASED", prevAgent, map[string]interface{}{
			"task_id": target.ID,
		})

		success = true
		resultMsg = fmt.Sprintf("Task '%s' released back to pending", target.ID)
		return nil
	})

	return success, resultMsg, err
}

// DeleteTask removes a task by ID.
func DeleteTask(repoRoot, taskID string) (bool, error) {
	var found bool
	err := storage.WithFileLock(repoRoot, func() error {
		tasks, err := ListTasks(repoRoot, nil, nil)
		if err != nil {
			return err
		}

		var remaining []Task
		for _, t := range tasks {
			if strings.EqualFold(t.ID, taskID) {
				found = true
			} else {
				remaining = append(remaining, t)
			}
		}

		if !found {
			return nil
		}

		return storage.WriteJSONFile(repoRoot, config.TasksFile, remaining)
	})

	return found, err
}
