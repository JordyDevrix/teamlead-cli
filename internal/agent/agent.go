package agent

import (
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/storage"
)

const (
	StatusActive = "active"
	StatusIdle   = "idle"
	StatusDone   = "done"
)

// AgentRecord represents metadata and runtime state for an agent.
type AgentRecord struct {
	Name          string  `json:"name"`
	Tool          *string `json:"tool"`
	Role          *string `json:"role"`
	WorktreePath  *string `json:"worktree_path"`
	CurrentBranch *string `json:"current_branch"`
	CurrentTaskID *string `json:"current_task_id"`
	RegisteredAt  string  `json:"registered_at"`
	LastHeartbeat string  `json:"last_heartbeat"`
	Status        string  `json:"status"`
	PID           *int    `json:"pid"`
}

// IsProcessRunning checks if the assigned PID exists on this system.
func (a *AgentRecord) IsProcessRunning() bool {
	if a.PID == nil || *a.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(*a.PID)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// MinutesSinceHeartbeat returns elapsed minutes since the last recorded heartbeat.
func (a *AgentRecord) MinutesSinceHeartbeat() float64 {
	t, err := time.Parse(time.RFC3339, a.LastHeartbeat)
	if err != nil {
		return 9999.0
	}
	diff := time.Since(t)
	mins := diff.Minutes()
	if mins < 0 {
		return 0
	}
	return mins
}

// ListAgents retrieves all registered agents from storage.
func ListAgents(repoRoot string) ([]AgentRecord, error) {
	var agents []AgentRecord
	if err := storage.ReadJSONFile(repoRoot, config.AgentsFile, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// GetAgent looks up an agent by name (case-insensitive).
func GetAgent(repoRoot, name string) (*AgentRecord, error) {
	agents, err := ListAgents(repoRoot)
	if err != nil {
		return nil, err
	}

	for _, a := range agents {
		if strings.EqualFold(a.Name, name) {
			copy := a
			return &copy, nil
		}
	}
	return nil, nil
}

// RegisterAgent adds or updates an agent record.
func RegisterAgent(repoRoot, name string, tool, role, worktreePath, currentBranch, currentTaskID *string) (*AgentRecord, error) {
	var target *AgentRecord
	cleanName := strings.TrimSpace(name)

	err := storage.WithFileLock(repoRoot, func() error {
		agents, err := ListAgents(repoRoot)
		if err != nil {
			return err
		}

		nowStr := time.Now().UTC().Format(time.RFC3339)
		idx := -1
		for i, a := range agents {
			if strings.EqualFold(a.Name, cleanName) {
				idx = i
				break
			}
		}

		if idx >= 0 {
			a := &agents[idx]
			if tool != nil {
				a.Tool = tool
			}
			if role != nil {
				a.Role = role
			}
			if worktreePath != nil {
				a.WorktreePath = worktreePath
			}
			if currentBranch != nil {
				a.CurrentBranch = currentBranch
			}
			if currentTaskID != nil {
				a.CurrentTaskID = currentTaskID
			}
			a.LastHeartbeat = nowStr
			a.Status = StatusActive
			target = a
		} else {
			a := AgentRecord{
				Name:          cleanName,
				Tool:          tool,
				Role:          role,
				WorktreePath:  worktreePath,
				CurrentBranch: currentBranch,
				CurrentTaskID: currentTaskID,
				RegisteredAt:  nowStr,
				LastHeartbeat: nowStr,
				Status:        StatusIdle,
			}
			agents = append(agents, a)
			target = &agents[len(agents)-1]
		}

		if err := storage.WriteJSONFile(repoRoot, config.AgentsFile, agents); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "AGENT_REGISTERED", &target.Name, map[string]interface{}{
			"tool": target.Tool,
			"role": target.Role,
		})

		return nil
	})

	return target, err
}

// HeartbeatAgent updates an agent's last heartbeat and refreshes its locks.
func HeartbeatAgent(repoRoot, name string, pid *int, taskID, branch *string, status string) (*AgentRecord, error) {
	var target *AgentRecord

	err := storage.WithFileLock(repoRoot, func() error {
		agents, err := ListAgents(repoRoot)
		if err != nil {
			return err
		}

		idx := -1
		for i, a := range agents {
			if strings.EqualFold(a.Name, name) {
				idx = i
				break
			}
		}

		if idx < 0 {
			return nil
		}

		a := &agents[idx]
		nowStr := time.Now().UTC().Format(time.RFC3339)
		a.LastHeartbeat = nowStr
		if status != "" {
			a.Status = status
		}
		if pid != nil {
			a.PID = pid
		}
		if taskID != nil {
			a.CurrentTaskID = taskID
		}
		if branch != nil {
			a.CurrentBranch = branch
		}

		if err := storage.WriteJSONFile(repoRoot, config.AgentsFile, agents); err != nil {
			return err
		}

		target = a
		return nil
	})

	if err != nil {
		return nil, err
	}

	if target != nil {
		_, _ = lock.RefreshAgentLocks(repoRoot, target.Name, nil)
	}

	return target, nil
}

// UnregisterAgent removes an agent, releases all locks, and optionally removes its worktree.
func UnregisterAgent(repoRoot, name string, cleanupWorktree bool) (bool, error) {
	var found bool

	err := storage.WithFileLock(repoRoot, func() error {
		agents, err := ListAgents(repoRoot)
		if err != nil {
			return err
		}

		var target *AgentRecord
		var remaining []AgentRecord

		for _, a := range agents {
			if strings.EqualFold(a.Name, name) {
				copy := a
				target = &copy
				found = true
			} else {
				remaining = append(remaining, a)
			}
		}

		if !found {
			return nil
		}

		// Release all locks
		_, _ = lock.ReleaseAllForAgent(repoRoot, target.Name)

		// Remove worktree
		if cleanupWorktree && target.WorktreePath != nil && *target.WorktreePath != "" {
			_ = git.RemoveWorktree(repoRoot, *target.WorktreePath, true)
		}

		if err := storage.WriteJSONFile(repoRoot, config.AgentsFile, remaining); err != nil {
			return err
		}

		storage.LogEvent(repoRoot, "AGENT_UNREGISTERED", &name, map[string]interface{}{
			"cleaned_worktree": cleanupWorktree,
		})

		return nil
	})

	return found, err
}
