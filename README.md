# 🛡️ teamlead-cli

> **Conflict-free multi-agent coordinator and workspace isolation CLI for AI coding agents.**  
> *Fast, lightweight, single-binary port in Go with a clean, low-cognitive-load terminal UI.*

`teamlead-cli` allows multiple different AI agents, models, and CLI tools (e.g. **Claude Code**, **Aider**, **Cursor**, **Copilot**, **Gemini CLI**, or custom LLM scripts) to collaborate concurrently on the same codebase on a single machine without stepping on each other, clobbering files, or breaking Git staging.

---

## 🌟 Why teamlead-cli?

- ⚡ **Zero Dependencies & Single Binary**: Fast startup, zero Python runtime overhead, and single-binary deployment (`go install` or direct binary).
- 🧠 **Low Cognitive Load UI**: Redesigned from the ground up for developer ergonomics. High visual noise and heavy stacked box borders have been replaced with a clean, minimal visual hierarchy that lets you assess status and collisions in under a second.
- 🌳 **Git Worktree Isolation**: Every agent automatically runs in its own dedicated, isolated Git worktree (`.teamlead/worktrees/<agent_name>`). Each agent has its own filesystem view, git index, unstaged changes, and branch.
- 🔒 **Advisory File Leases & Locks**: Path-level and glob-level reservation (`teamlead lock acquire "src/auth/**"`). Prevents agents from claiming overlapping files before work begins. Heartbeat threads prevent deadlocks if an agent hangs or crashes.
- 📋 **Scoped Task Coordination**: Assign tasks with explicit file scopes (`teamlead task add "Fix auth" --scope "src/auth/**"`). Claiming a task automatically locks its declared files.
- 🚀 **Transparent Agent Wrapper (`teamlead run`)**: Run any CLI tool or command through `teamlead`. It automatically manages the worktree, acquires locks, injects collaboration context, keeps the heartbeat alive, and summarizes changes upon exit.
- 🔍 **Multi-Agent Conflict Detector (`teamlead conflicts`)**: Inspects uncommitted changes and committed branch diffs across all active agents simultaneously, flagging direct file collisions, lock violations, and stale branches before merging.
- 🔀 **Automated Merge Queue (`teamlead merge`)**: Validates that the worktree is clean, runs pre-merge test commands (e.g. `go test`, `pytest`, `npm test`), cleanly merges into the base branch (`squash`, `rebase`, or `merge`), releases locks, and marks the task as completed.

---

## 🖥️ Clean, Low-Cognitive-Load Dashboard

When running `teamlead status` or `teamlead monitor`:

```text
🛡️  TEAMLEAD  ·  my-project
   base: main  ·  strategy: squash  ·  active agents: 2
────────────────────────────────────────────────────────────────────
 ●  System healthy — all isolated branches conflict-free

COLLABORATORS
   ● claude        claude (PID 4920)   branch: teamlead/claude/T-1  task: T-1      active (just now)
   ○ aider         aider               branch: teamlead/aider/T-2   task: T-2      idle (4m ago)

TASK BOARD
   [T-1]  ● in progress  Implement user authentication (claude)  scope: src/auth/**
   [T-2]  ○ pending      Implement billing and invoices          scope: src/billing/**
   [T-3]  ✔ done         Documentation and OpenAPI setup         scope: docs/**

ACTIVE FILE LEASES
   🔒 src/auth/**                held by claude  52m remaining [T-1]

WORKTREE SYNC
   claude        teamlead/claude/T-1       synced          clean
   aider         teamlead/aider/T-2        ↑1 ahead        dirty (1 files)

RECENT ACTIVITY
   19:04:12  claude      LOCK_ACQUIRED       paths=[src/auth/**] task_id=T-1
   19:04:02  claude      TASK_CLAIMED        task_id=T-1 title=Implement user auth
────────────────────────────────────────────────────────────────────
```

---

## 📦 Installation & Build

### From Source
```bash
git clone https://github.com/JordyDevrix/teamlead-cli.git
cd teamlead-cli
make build
# Binary is built at ./teamlead-cli (and symlinked as ./teamlead)
```

### Install to `$GOPATH/bin`
```bash
make install
```

Verify installation:
```bash
teamlead --version
```

---

## ⚡ Quickstart Tutorial

### 1. Initialize a Repository
Navigate to any git repository and run:
```bash
teamlead init
```
This creates `.teamlead/`, sets up the base branch, updates `.gitignore`, and generates `AGENTS.md` for AI models.

### 2. Define Tasks with Scopes
Define what work needs to be done and establish file boundaries:
```bash
teamlead task add "Implement user authentication" --scope "src/auth/**"
teamlead task add "Implement billing and invoices" --scope "src/billing/**"
```

### 3. Run Agents Concurrently in Parallel Terminals

**Terminal 1 (Running Claude Code):**
```bash
teamlead run --agent claude --task T-1 -- claude
```

**Terminal 2 (Running Aider):**
```bash
teamlead run --agent aider --task T-2 -- aider
```

**Terminal 3 (Running custom agent or script):**
```bash
teamlead run --agent bot-3 --task T-3 -- python3 my_agent.py
```

Each agent executes inside its own dedicated git worktree with isolated git indexes. They can create files, run local tests, and commit changes without interfering with one another!

### 4. Check the Live Dashboard
```bash
teamlead status
# Or watch it refresh continuously:
teamlead monitor
```

### 5. Check for Conflicts
Before merging, verify that no agents collided on the same files:
```bash
teamlead conflicts
```

### 6. Cleanly Integrate Work
Once an agent finishes its task, integrate its changes into the main branch:
```bash
teamlead merge claude
teamlead merge aider
```

`teamlead` validates that tests pass, cleanly merges into `main`, releases all held locks, marks the tasks as `done`, and deletes the feature branches.

---

## 📖 CLI Command Reference

### Project Setup & Monitoring
| Command | Description |
|---|---|
| `teamlead init [--base-branch <branch>] [--strategy <squash\|rebase\|merge>] [--test-cmd <cmd>]` | Initialize teamlead in the current repo |
| `teamlead status` | Display the real-time multi-agent dashboard |
| `teamlead monitor [-i <seconds>]` | Continuously refresh live coordination dashboard |
| `teamlead conflicts` | Analyze all active branches and locks for collisions |

### Agent Execution & Management
| Command | Description |
|---|---|
| `teamlead run --agent <name> [--task <id>] [--lock <path...>] -- <cmd...>` | Run an agent command in an isolated worktree with file locking |
| `teamlead agent list` | List all registered agents and their current statuses |
| `teamlead agent register <name> [--tool <tool>] [--role <role>]` | Manually register an agent |
| `teamlead agent unregister <name> [--keep-worktree]` | Unregister an agent and release its locks |

### Task Coordination
| Command | Description |
|---|---|
| `teamlead task add <title> [--desc <desc>] [--scope <path...>]` | Create a new task with declared file scopes |
| `teamlead task list [--status <status>] [--agent <agent>]` | List tasks on the board |
| `teamlead task claim <task_id> --agent <name>` | Claim a task and reserve its declared scope |
| `teamlead task complete <task_id> [--agent <name>]` | Mark task as done and release locks |
| `teamlead task release <task_id>` | Release a claimed task back to pending |
| `teamlead task delete <task_id>` | Delete a task |

### File Locks & Leases
| Command | Description |
|---|---|
| `teamlead lock list` | List all active file leases and expiration times |
| `teamlead lock acquire <path...> --agent <name> [--ttl <mins>]` | Acquire exclusive lease on files or glob patterns |
| `teamlead lock check <path> [--agent <name>]` | Check whether a file is currently locked or free |
| `teamlead lock release <path...> --agent <name>` | Release specific file locks |
| `teamlead lock release-all --agent <name>` | Release all locks held by an agent |

### Worktree & Integration
| Command | Description |
|---|---|
| `teamlead worktree list` | List active git worktrees |
| `teamlead worktree create <agent> [--branch <name>] [--base <branch>]` | Create a worktree manually |
| `teamlead worktree remove <agent> [--force]` | Remove an agent's worktree |
| `teamlead sync --agent <name> [--strategy <rebase\|merge>]` | Sync / rebase an agent's branch against the base branch |
| `teamlead merge <agent> [--strategy <squash\|rebase\|merge>]` | Validate, test, and merge agent branch into base branch |

### System Prompts for AI Models
| Command | Description |
|---|---|
| `teamlead guide [--format claude\|cursor\|gemini\|markdown] [--write]` | Generate instructions or system prompt files (`CLAUDE.md`, `.cursorrules`, etc.) |

---

## 🧪 Testing

Run the test suite:
```bash
make test
# Or using go test:
go test -v ./...
```
