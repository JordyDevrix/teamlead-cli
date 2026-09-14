package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/JordyDevrix/teamlead-cli/internal/agent"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/conflict"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/lock"
	"github.com/JordyDevrix/teamlead-cli/internal/storage"
	"github.com/JordyDevrix/teamlead-cli/internal/task"
)

var (
	green   = color.New(color.FgGreen).SprintFunc()
	bGreen  = color.New(color.FgGreen, color.Bold).SprintFunc()
	red     = color.New(color.FgRed).SprintFunc()
	bRed    = color.New(color.FgRed, color.Bold).SprintFunc()
	yellow  = color.New(color.FgYellow).SprintFunc()
	bYellow = color.New(color.FgYellow, color.Bold).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
	bCyan   = color.New(color.FgCyan, color.Bold).SprintFunc()
	dim     = color.New(color.Faint).SprintFunc()
	bold    = color.New(color.Bold).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
)

// Success prints a green checkmark message.
func Success(msg string) {
	fmt.Printf("%s %s\n", bGreen("✔"), msg)
}

// Successf prints a formatted green checkmark message.
func Successf(format string, a ...interface{}) {
	fmt.Printf("%s %s\n", bGreen("✔"), fmt.Sprintf(format, a...))
}

// Info prints a cyan info message.
func Info(msg string) {
	fmt.Printf("%s %s\n", bCyan("ℹ"), msg)
}

// Infof prints a formatted cyan info message.
func Infof(format string, a ...interface{}) {
	fmt.Printf("%s %s\n", bCyan("ℹ"), fmt.Sprintf(format, a...))
}

// Warning prints a yellow warning message.
func Warning(msg string) {
	fmt.Printf("%s %s\n", bYellow("⚠"), msg)
}

// Warningf prints a formatted yellow warning message.
func Warningf(format string, a ...interface{}) {
	fmt.Printf("%s %s\n", bYellow("⚠"), fmt.Sprintf(format, a...))
}

// Error prints a red error message.
func Error(msg string) {
	fmt.Printf("%s %s\n", bRed("✖"), msg)
}

// Errorf prints a formatted red error message.
func Errorf(format string, a ...interface{}) {
	fmt.Printf("%s %s\n", bRed("✖"), fmt.Sprintf(format, a...))
}

// RenderDashboard renders a clean, uncluttered, low-cognitive-load status overview.
func RenderDashboard(repoRoot string) error {
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return err
	}

	agents, err := agent.ListAgents(repoRoot)
	if err != nil {
		return err
	}

	tasks, err := task.ListTasks(repoRoot, nil, nil)
	if err != nil {
		return err
	}

	activeLocks, err := lock.GetActiveLocks(repoRoot, true)
	if err != nil {
		return err
	}

	conflicts, err := conflict.DetectConflicts(repoRoot)
	if err != nil {
		return err
	}

	events, _ := storage.ReadRecentEvents(repoRoot, 4)

	repoName := filepath.Base(repoRoot)
	divider := dim(strings.Repeat("─", 68))

	// 1. Header Banner
	fmt.Println()
	fmt.Printf("%s  %s  %s\n",
		bCyan("🛡️  TEAMLEAD"),
		dim("·"),
		bold(repoName),
	)
	fmt.Printf("   %s %s  %s  %s %s  %s  %s %s\n",
		dim("base:"), bGreen(cfg.BaseBranch),
		dim("·"),
		dim("strategy:"), cyan(cfg.MergeStrategy),
		dim("·"),
		dim("active agents:"), bold(fmt.Sprintf("%d", len(agents))),
	)
	fmt.Println(divider)

	// 2. Conflicts or Health check
	if conflicts.HasBlockingConflicts || len(conflicts.StaleBranchWarnings) > 0 {
		fmt.Printf("\n%s\n", bRed("⚠  ATTENTION REQUIRED: ACTIVE CONFLICTS & WARNINGS"))
		for _, c := range conflicts.DirectFileConflicts {
			fmt.Printf("   %s Direct Collision on %s (modified by both %s and %s)\n",
				bRed("✖"), bold(c.File), cyan(c.Agents[0]), cyan(c.Agents[1]))
		}
		for _, v := range conflicts.LockViolations {
			fmt.Printf("   %s Lock Violation on %s (%s modified file locked by %s)\n",
				bRed("✖"), bold(v.File), cyan(v.ModifyingAgent), yellow(v.LockingAgent))
		}
		for _, s := range conflicts.StaleBranchWarnings {
			fmt.Printf("   %s Stale Branch: %s (%s) is %s behind %s\n",
				yellow("⚠"), bold(s.Branch), cyan(s.Agent), bRed(fmt.Sprintf("%d commits", s.Behind)), cfg.BaseBranch)
		}
		for _, o := range conflicts.ScopeOverlapWarnings {
			fmt.Printf("   %s Scope Overlap: %s (%s) and %s (%s) overlap on '%s'\n",
				yellow("⚠"), magenta(o.Task1), cyan(o.Agent1), magenta(o.Task2), cyan(o.Agent2), o.Scope1)
		}
		if len(conflicts.Recommendations) > 0 {
			fmt.Println(dim("\n   Recommended action:"))
			for _, rec := range conflicts.Recommendations {
				fmt.Printf("   %s %s\n", bCyan("→"), rec)
			}
		}
		fmt.Println()
	} else {
		fmt.Printf(" %s  %s\n", bGreen("●"), green("System healthy — all isolated branches conflict-free"))
	}

	// 3. Active Collaborators
	fmt.Println()
	fmt.Println(bold("COLLABORATORS"))
	if len(agents) == 0 {
		fmt.Println(dim("   No agents registered. Run 'teamlead run' or 'teamlead agent register'."))
	} else {
		for _, a := range agents {
			dot := bGreen("●")
			statusText := green("active")
			if a.Status == agent.StatusIdle {
				dot = yellow("○")
				statusText = yellow("idle")
			}

			mins := a.MinutesSinceHeartbeat()
			timeStr := "just now"
			if mins >= 60 {
				timeStr = fmt.Sprintf("%dh ago", int(mins/60))
			} else if mins >= 1 {
				timeStr = fmt.Sprintf("%dm ago", int(mins))
			}

			toolStr := "agent"
			if a.Tool != nil && *a.Tool != "" {
				toolStr = *a.Tool
			}
			if a.PID != nil && a.IsProcessRunning() {
				toolStr += fmt.Sprintf(" (PID %d)", *a.PID)
			}

			branchStr := "-"
			if a.CurrentBranch != nil && *a.CurrentBranch != "" {
				branchStr = *a.CurrentBranch
			}

			taskStr := "-"
			if a.CurrentTaskID != nil && *a.CurrentTaskID != "" {
				taskStr = magenta(*a.CurrentTaskID)
			}

			fmt.Printf("   %s %-12s %-18s branch: %-22s task: %-8s %s (%s)\n",
				dot,
				bCyan(a.Name),
				dim(toolStr),
				cyan(branchStr),
				taskStr,
				statusText,
				dim(timeStr),
			)
		}
	}

	// 4. Task Board
	fmt.Println()
	fmt.Println(bold("TASK BOARD"))
	if len(tasks) == 0 {
		fmt.Println(dim("   No tasks defined. Run 'teamlead task add <TITLE>' to create one."))
	} else {
		for _, t := range tasks {
			badge := dim("○ pending   ")
			if t.Status == task.StatusInProgress {
				badge = bYellow("● in progress")
			} else if t.Status == task.StatusDone {
				badge = bGreen("✔ done       ")
			} else if t.Status == task.StatusReview {
				badge = bCyan("◆ review     ")
			}

			agentStr := ""
			if t.AssignedAgent != nil && *t.AssignedAgent != "" {
				agentStr = fmt.Sprintf(" (%s)", cyan(*t.AssignedAgent))
			}

			scopeStr := ""
			if len(t.Scope) > 0 {
				scopeStr = dim(fmt.Sprintf("  scope: %s", strings.Join(t.Scope, ", ")))
			}

			fmt.Printf("   [%s]  %s  %s%s%s\n",
				magenta(t.ID),
				badge,
				t.Title,
				agentStr,
				scopeStr,
			)
		}
	}

	// 5. Active File Locks (only shown if there are locks)
	if len(activeLocks) > 0 {
		fmt.Println()
		fmt.Println(bold("ACTIVE FILE LEASES"))
		for _, l := range activeLocks {
			rem := int(l.RemainingMinutes())
			remStr := fmt.Sprintf("%dm remaining", rem)
			if rem < 1 {
				remStr = "<1m remaining"
			}
			taskStr := ""
			if l.TaskID != nil && *l.TaskID != "" {
				taskStr = fmt.Sprintf(" [%s]", magenta(*l.TaskID))
			}
			fmt.Printf("   🔒 %-26s %s %s  %s%s\n",
				bYellow(l.Path),
				dim("held by"),
				cyan(l.Agent),
				dim(remStr),
				taskStr,
			)
		}
	}

	// 6. Git Worktrees & Branch Sync
	hasWorktrees := false
	for _, a := range agents {
		if a.WorktreePath != nil && *a.WorktreePath != "" {
			if _, err := os.Stat(*a.WorktreePath); err == nil {
				hasWorktrees = true
				break
			}
		}
	}

	if hasWorktrees {
		fmt.Println()
		fmt.Println(bold("WORKTREE SYNC"))
		for _, a := range agents {
			if a.WorktreePath == nil || *a.WorktreePath == "" {
				continue
			}
			st, err := git.GetWorktreeStatus(*a.WorktreePath)
			if err != nil || !st.Exists {
				continue
			}

			branch := st.Branch
			if branch == "" && a.CurrentBranch != nil {
				branch = *a.CurrentBranch
			}

			behind, ahead, _ := git.GetBranchDivergence(repoRoot, branch, cfg.BaseBranch)
			divBadge := bGreen("synced")
			if behind > 0 && ahead > 0 {
				divBadge = fmt.Sprintf("%s %s", bRed(fmt.Sprintf("↓%d", behind)), bGreen(fmt.Sprintf("↑%d", ahead)))
			} else if behind > 0 {
				divBadge = bRed(fmt.Sprintf("↓%d behind", behind))
			} else if ahead > 0 {
				divBadge = bGreen(fmt.Sprintf("↑%d ahead", ahead))
			}

			dirtyBadge := green("clean")
			if st.IsDirty {
				dirtyBadge = bYellow(fmt.Sprintf("dirty (%d files)", len(st.AllDirtyFiles)))
			}

			fmt.Printf("   %-12s  %-24s  %-14s  %s\n",
				cyan(a.Name),
				dim(branch),
				divBadge,
				dirtyBadge,
			)
		}
	}

	// 7. Recent Activity (compact, last 4)
	if len(events) > 0 {
		fmt.Println()
		fmt.Println(bold("RECENT ACTIVITY"))
		for _, ev := range events {
			ts := ev.Timestamp
			if len(ts) >= 19 {
				ts = ts[11:19]
			}
			agentName := "system"
			if ev.Agent != nil && *ev.Agent != "" {
				agentName = *ev.Agent
			}
			detailsList := []string{}
			for k, v := range ev.Details {
				detailsList = append(detailsList, fmt.Sprintf("%s=%v", k, v))
				if len(detailsList) >= 2 {
					break
				}
			}
			detailStr := strings.Join(detailsList, " ")
			fmt.Printf("   %s  %-10s  %-18s  %s\n",
				dim(ts),
				cyan(agentName),
				bold(ev.Event),
				dim(detailStr),
			)
		}
	}

	fmt.Println(divider)
	fmt.Println()
	return nil
}
