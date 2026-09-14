package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
)

// GenerateCollaborationContext builds markdown briefing for an agent inside its worktree.
func GenerateCollaborationContext(repoRoot, agentName string) (string, error) {
	ag, err := agent.GetAgent(repoRoot, agentName)
	if err != nil {
		return "", err
	}

	allAgents, err := agent.ListAgents(repoRoot)
	if err != nil {
		return "", err
	}

	activeLocks, err := lock.GetActiveLocks(repoRoot, true)
	if err != nil {
		return "", err
	}

	var currentTask *task.Task
	if ag != nil && ag.CurrentTaskID != nil && *ag.CurrentTaskID != "" {
		currentTask, _ = task.GetTask(repoRoot, *ag.CurrentTaskID)
	}

	var otherAgents []agent.AgentRecord
	for _, a := range allAgents {
		if !strings.EqualFold(a.Name, agentName) {
			otherAgents = append(otherAgents, a)
		}
	}

	var b strings.Builder
	nowUTC := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

	b.WriteString(fmt.Sprintf("# TeamLead Agent Briefing: %s\n", agentName))
	b.WriteString(fmt.Sprintf("*Generated on %s*\n\n", nowUTC))
	b.WriteString("You are working as part of a collaborative multi-agent development team coordinated by `teamlead`.\n\n")

	b.WriteString("## Your Assignment\n")
	if currentTask != nil {
		desc := "No description provided."
		if currentTask.Description != nil && *currentTask.Description != "" {
			desc = *currentTask.Description
		}
		b.WriteString(fmt.Sprintf("- **Task ID**: `%s`\n", currentTask.ID))
		b.WriteString(fmt.Sprintf("- **Title**: %s\n", currentTask.Title))
		b.WriteString(fmt.Sprintf("- **Status**: `%s`\n", currentTask.Status))
		b.WriteString(fmt.Sprintf("- **Description**: %s\n", desc))
		b.WriteString("- **Scoped Files / Directories**:\n")
		if len(currentTask.Scope) > 0 {
			for _, s := range currentTask.Scope {
				b.WriteString(fmt.Sprintf("  - `%s`\n", s))
			}
		} else {
			b.WriteString("  - *No specific scope restricted (please minimize unnecessary changes)*\n")
		}
	} else {
		b.WriteString("- *No specific task claimed yet. Run `teamlead task claim <TASK_ID>` to claim work.*\n")
	}

	b.WriteString("\n## Active Collaborators on This Repository\n")
	if len(otherAgents) > 0 {
		for _, oa := range otherAgents {
			tool := "AI"
			if oa.Tool != nil && *oa.Tool != "" {
				tool = *oa.Tool
			}
			taskStr := "Idle"
			if oa.CurrentTaskID != nil && *oa.CurrentTaskID != "" {
				taskStr = fmt.Sprintf("Task: %s", *oa.CurrentTaskID)
			}
			b.WriteString(fmt.Sprintf("- **Agent `%s`** (%s) — %s (%s)\n", oa.Name, tool, oa.Status, taskStr))
		}
	} else {
		b.WriteString("- No other agents currently registered.\n")
	}

	b.WriteString("\n## Currently Locked Files (DO NOT MODIFY)\n")
	var lockedByOthers []lock.FileLock
	for _, l := range activeLocks {
		if !strings.EqualFold(l.Agent, agentName) {
			lockedByOthers = append(lockedByOthers, l)
		}
	}

	if len(lockedByOthers) > 0 {
		for _, l := range lockedByOthers {
			expShort := l.ExpiresAt
			if len(expShort) >= 19 {
				expShort = expShort[:19] + "Z"
			}
			b.WriteString(fmt.Sprintf("- `%s` — locked by **%s** (until %s)\n", l.Path, l.Agent, expShort))
		}
	} else {
		b.WriteString("- No files are currently locked by other agents.\n")
	}

	b.WriteString("\n## Golden Rules for Conflict-Free Collaboration\n")
	b.WriteString("1. **Stay in Scope**: Only create or modify files relevant to your task.\n")
	b.WriteString("2. **Check Locks**: Never write to files locked by another agent.\n")
	b.WriteString("3. **Atomic Commits**: Commit your changes frequently with clear, descriptive commit messages.\n")
	b.WriteString("4. **Don't Mess with .teamlead/ metadata**: The teamlead CLI handles locks and tracking automatically.\n")
	b.WriteString("5. **When Finished**: Ensure your tests pass and signal completion so teamlead can cleanly merge your work.\n\n")

	return b.String(), nil
}

// InjectContextIntoWorktree creates COLLABORATION.md in an agent's worktree.
func InjectContextIntoWorktree(repoRoot, worktreePath, agentName string) (string, error) {
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		return "", err
	}

	content, err := GenerateCollaborationContext(repoRoot, agentName)
	if err != nil {
		return "", err
	}

	target := filepath.Join(worktreePath, "COLLABORATION.md")
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return "", err
	}

	return target, nil
}

// UpdateRootAgentsMD updates AGENTS.md in the root repository for all LLMs to read.
func UpdateRootAgentsMD(repoRoot string) (string, error) {
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return "", err
	}

	tasks, err := task.ListTasks(repoRoot, nil, nil)
	if err != nil {
		return "", err
	}

	activeLocks, err := lock.GetActiveLocks(repoRoot, true)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# TeamLead Coordination System\n\n")
	b.WriteString("This repository uses **teamlead** to coordinate multiple AI models, agents, and CLI tools simultaneously.\n\n")

	b.WriteString("## Project Configuration\n")
	b.WriteString(fmt.Sprintf("- **Base Branch**: `%s`\n", cfg.BaseBranch))
	b.WriteString(fmt.Sprintf("- **Merge Strategy**: `%s`\n", cfg.MergeStrategy))
	b.WriteString(fmt.Sprintf("- **Worktree Directory**: `%s`\n\n", cfg.WorktreeDir))

	b.WriteString("## CLI Quick Reference for AI Agents\n")
	b.WriteString("```bash\n")
	b.WriteString("# 1. Check current tasks and locks\n")
	b.WriteString("teamlead status\n")
	b.WriteString("teamlead task list\n")
	b.WriteString("teamlead lock list\n\n")
	b.WriteString("# 2. Claim your task and reserve files\n")
	b.WriteString("teamlead task claim T-1 --agent <YOUR_NAME>\n")
	b.WriteString("teamlead lock acquire src/feature/* --agent <YOUR_NAME>\n\n")
	b.WriteString("# 3. Check for conflicts before committing\n")
	b.WriteString("teamlead conflicts\n\n")
	b.WriteString("# 4. Complete task\n")
	b.WriteString("teamlead task complete T-1 --agent <YOUR_NAME>\n")
	b.WriteString("```\n\n")

	b.WriteString("## Current Tasks\n")
	if len(tasks) > 0 {
		for _, t := range tasks {
			assigned := "none"
			if t.AssignedAgent != nil && *t.AssignedAgent != "" {
				assigned = *t.AssignedAgent
			}
			scopeStr := ""
			if len(t.Scope) > 0 {
				scopeStr = fmt.Sprintf(" [scope: %s]", strings.Join(t.Scope, ", "))
			}
			b.WriteString(fmt.Sprintf("- `[%s]` (%s) %s - assigned: %s%s\n", t.ID, t.Status, t.Title, assigned, scopeStr))
		}
	} else {
		b.WriteString("- *No tasks created yet.*\n")
	}

	b.WriteString("\n## Active Locks\n")
	if len(activeLocks) > 0 {
		for _, l := range activeLocks {
			b.WriteString(fmt.Sprintf("- `%s` locked by `%s`\n", l.Path, l.Agent))
		}
	} else {
		b.WriteString("- *No active locks.*\n")
	}
	b.WriteString("\n")

	target := filepath.Join(repoRoot, "AGENTS.md")
	if err := os.WriteFile(target, []byte(b.String()), 0644); err != nil {
		return "", err
	}

	return target, nil
}
