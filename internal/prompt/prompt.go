package prompt

import "fmt"

// GetAgentRulesText returns coordination instructions for AI prompts.
func GetAgentRulesText(toolName string) string {
	name := toolName
	if name == "" {
		name = "general"
	}

	return fmt.Sprintf(`# TeamLead Autonomous Multi-Agent Protocol

You are operating inside a collaborative repository managed by **teamlead**.
Multiple AI agents (Claude, Aider, Cursor, Copilot, Gemini, etc.) may be working concurrently on this codebase.

## Mandatory Coordination Rules:
1. **Never edit files outside your assigned task or scope.**
2. **Advisory Locking**:
   - Before modifying any file or module, check whether it is locked:
     `+"`teamlead lock check <file_path>`"+`
   - If you need exclusive access to a file or module, acquire a lock:
     `+"`teamlead lock acquire <path_or_glob> --agent <YOUR_NAME> --reason \"<what you are doing>\"`"+`
   - Release the lock when you finish or let `+"`teamlead merge`"+` handle it automatically.
3. **Workspace Isolation**:
   - You should do your edits inside your assigned worktree.
   - If running inside a worktree, never touch files outside that worktree.
4. **Git Hygiene**:
   - Make small, atomic commits with clear commit messages.
   - Do not force-push, do not tamper with .git/index or other worktrees.
   - Check `+"`teamlead conflicts`"+` if you suspect collisions.
5. **Task Lifecycle**:
   - Inspect tasks: `+"`teamlead task list`"+`
   - Claim task: `+"`teamlead task claim <TASK_ID> --agent <YOUR_NAME>`"+`
   - Signal completion: `+"`teamlead task complete <TASK_ID> --agent <YOUR_NAME>`"+`
`)
}

// GenerateClaudeMD returns CLAUDE.md content.
func GenerateClaudeMD() string {
	return GetAgentRulesText("Claude Code")
}

// GenerateCursorRules returns .cursorrules content.
func GenerateCursorRules() string {
	return GetAgentRulesText("Cursor")
}

// GenerateGeminiMD returns GEMINI.md content.
func GenerateGeminiMD() string {
	return GetAgentRulesText("Gemini")
}
