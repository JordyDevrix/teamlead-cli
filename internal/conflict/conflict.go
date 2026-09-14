package conflict

import (
	"fmt"
	"sort"
	"strings"

	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
)

// DirectConflict describes two agents modifying the same file.
type DirectConflict struct {
	File    string   `json:"file"`
	Agents  []string `json:"agents"`
	Branch1 string   `json:"branch_1"`
	Branch2 string   `json:"branch_2"`
}

// LockViolation describes an agent modifying a file locked by another agent.
type LockViolation struct {
	File           string  `json:"file"`
	ModifyingAgent string  `json:"modifying_agent"`
	LockingAgent   string  `json:"locking_agent"`
	LockPattern    string  `json:"lock_pattern"`
	TaskID         *string `json:"task_id"`
}

// StaleBranchWarning describes a branch that has fallen behind the base branch.
type StaleBranchWarning struct {
	Agent  string `json:"agent"`
	Branch string `json:"branch"`
	Behind int    `json:"behind"`
	Ahead  int    `json:"ahead"`
}

// ScopeOverlapWarning describes two active tasks claiming overlapping scopes.
type ScopeOverlapWarning struct {
	Task1  string `json:"task_1"`
	Agent1 string `json:"agent_1"`
	Scope1 string `json:"scope_1"`
	Task2  string `json:"task_2"`
	Agent2 string `json:"agent_2"`
	Scope2 string `json:"scope_2"`
}

// ConflictReport summarizes collision analysis across all active worktrees and branches.
type ConflictReport struct {
	HasBlockingConflicts bool                  `json:"has_blocking_conflicts"`
	DirectFileConflicts  []DirectConflict      `json:"direct_file_conflicts"`
	LockViolations       []LockViolation       `json:"lock_violations"`
	StaleBranchWarnings  []StaleBranchWarning  `json:"stale_branch_warnings"`
	ScopeOverlapWarnings []ScopeOverlapWarning `json:"scope_overlap_warnings"`
	Recommendations      []string              `json:"recommendations"`
}

// DetectConflicts inspects all active worktrees, branches, and locks for collisions.
func DetectConflicts(repoRoot string) (*ConflictReport, error) {
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return nil, err
	}

	agents, err := agent.ListAgents(repoRoot)
	if err != nil {
		return nil, err
	}

	activeLocks, err := lock.GetActiveLocks(repoRoot, true)
	if err != nil {
		return nil, err
	}

	tasks, err := task.ListTasks(repoRoot, nil, nil)
	if err != nil {
		return nil, err
	}

	report := &ConflictReport{}

	// 1. Inspect files touched by each agent (uncommitted changes + committed branch diff)
	agentTouchedFiles := make(map[string]map[string]bool)
	agentBranches := make(map[string]string)

	for _, a := range agents {
		touched := make(map[string]bool)
		branch := ""
		if a.CurrentBranch != nil {
			branch = *a.CurrentBranch
		}

		if a.WorktreePath != nil && *a.WorktreePath != "" {
			wtStatus, err := git.GetWorktreeStatus(*a.WorktreePath)
			if err == nil && wtStatus.Exists {
				if wtStatus.Branch != "" {
					branch = wtStatus.Branch
				}
				for _, f := range wtStatus.AllDirtyFiles {
					touched[f] = true
				}
			}
		}

		if branch != "" {
			agentBranches[a.Name] = branch
			diffFiles, err := git.GetCommittedFilesDiff(repoRoot, branch, cfg.BaseBranch)
			if err == nil {
				for _, f := range diffFiles {
					touched[f] = true
				}
			}
		}

		// Filter transient files
		cleanMap := make(map[string]bool)
		for f := range touched {
			if f != "COLLABORATION.md" && !strings.HasPrefix(f, ".teamlead/") && f != ".DS_Store" {
				cleanMap[f] = true
			}
		}
		agentTouchedFiles[a.Name] = cleanMap
	}

	// 2. Check for direct collisions between agents
	var agentNames []string
	for name := range agentTouchedFiles {
		agentNames = append(agentNames, name)
	}
	sort.Strings(agentNames)

	for i := 0; i < len(agentNames); i++ {
		for j := i + 1; j < len(agentNames); j++ {
			a1 := agentNames[i]
			a2 := agentNames[j]

			var overlappingFiles []string
			for f := range agentTouchedFiles[a1] {
				if agentTouchedFiles[a2][f] {
					overlappingFiles = append(overlappingFiles, f)
				}
			}
			sort.Strings(overlappingFiles)

			if len(overlappingFiles) > 0 {
				report.HasBlockingConflicts = true
				for _, f := range overlappingFiles {
					report.DirectFileConflicts = append(report.DirectFileConflicts, DirectConflict{
						File:    f,
						Agents:  []string{a1, a2},
						Branch1: agentBranches[a1],
						Branch2: agentBranches[a2],
					})
					report.Recommendations = append(report.Recommendations, fmt.Sprintf(
						"Direct collision on '%s': both '%s' and '%s' modified it. Coordinate who merges first, then sync the other.",
						f, a1, a2,
					))
				}
			}
		}
	}

	// 3. Check for lock violations (agent modified a file reserved by another agent)
	for aName, files := range agentTouchedFiles {
		var sortedFiles []string
		for f := range files {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Strings(sortedFiles)

		for _, f := range sortedFiles {
			for _, l := range activeLocks {
				if l.Agent != aName && lock.PatternsOverlap(f, l.Path) {
					report.HasBlockingConflicts = true
					report.LockViolations = append(report.LockViolations, LockViolation{
						File:           f,
						ModifyingAgent: aName,
						LockingAgent:   l.Agent,
						LockPattern:    l.Path,
						TaskID:         l.TaskID,
					})
					report.Recommendations = append(report.Recommendations, fmt.Sprintf(
						"Lock violation: Agent '%s' modified '%s', which is locked by '%s' (pattern: %s).",
						aName, f, l.Agent, l.Path,
					))
				}
			}
		}
	}

	// 4. Check for stale branches (behind base branch)
	for aName, branch := range agentBranches {
		behind, ahead, err := git.GetBranchDivergence(repoRoot, branch, cfg.BaseBranch)
		if err == nil && behind > 0 {
			report.StaleBranchWarnings = append(report.StaleBranchWarnings, StaleBranchWarning{
				Agent:  aName,
				Branch: branch,
				Behind: behind,
				Ahead:  ahead,
			})
			report.Recommendations = append(report.Recommendations, fmt.Sprintf(
				"Branch '%s' (%s) is %d commits behind '%s'. Run 'teamlead sync --agent %s' to stay up to date.",
				branch, aName, behind, cfg.BaseBranch, aName,
			))
		}
	}

	// 5. Check for overlapping scopes in in-progress tasks
	var inProgressTasks []task.Task
	for _, t := range tasks {
		if t.Status == task.StatusInProgress && len(t.Scope) > 0 && t.AssignedAgent != nil {
			inProgressTasks = append(inProgressTasks, t)
		}
	}

	for i := 0; i < len(inProgressTasks); i++ {
		for j := i + 1; j < len(inProgressTasks); j++ {
			t1 := inProgressTasks[i]
			t2 := inProgressTasks[j]

			if t1.AssignedAgent != nil && t2.AssignedAgent != nil && *t1.AssignedAgent != *t2.AssignedAgent {
				for _, s1 := range t1.Scope {
					for _, s2 := range t2.Scope {
						if lock.PatternsOverlap(s1, s2) {
							report.ScopeOverlapWarnings = append(report.ScopeOverlapWarnings, ScopeOverlapWarning{
								Task1:  t1.ID,
								Agent1: *t1.AssignedAgent,
								Scope1: s1,
								Task2:  t2.ID,
								Agent2: *t2.AssignedAgent,
								Scope2: s2,
							})
							report.Recommendations = append(report.Recommendations, fmt.Sprintf(
								"Task scope overlap: %s (%s) and %s (%s) overlap between '%s' and '%s'.",
								t1.ID, s1, t2.ID, s2, *t1.AssignedAgent, *t2.AssignedAgent,
							))
						}
					}
				}
			}
		}
	}

	return report, nil
}
