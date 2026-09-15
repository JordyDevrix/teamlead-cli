package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// GitError represents an execution error from a git command.
type GitError struct {
	Message    string
	ExitCode   int
	Stdout     string
	Stderr     string
	CommandLine []string
}

func (e *GitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(e.Stdout)
	}
	if msg == "" {
		msg = e.Message
	}
	return fmt.Sprintf("git error (exit %d): %s", e.ExitCode, msg)
}

// RunGit executes a git command in the specified directory and returns stdout.
func RunGit(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		exitCode := 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return "", &GitError{
			Message:     err.Error(),
			ExitCode:    exitCode,
			Stdout:      stdout.String(),
			Stderr:      stderr.String(),
			CommandLine: append([]string{"git"}, args...),
		}
	}

	return stdout.String(), nil
}

// IsGitRepo returns true if the directory is inside a git work tree.
func IsGitRepo(path string) bool {
	out, err := RunGit(path, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// GetRepoRoot returns the absolute path of the top-level repository.
func GetRepoRoot(path string) (string, error) {
	out, err := RunGit(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(out)
	if res == "" {
		return "", errors.New("empty repository root path")
	}
	return filepath.Clean(res), nil
}

// GetCurrentBranch returns the active branch name in cwd.
func GetCurrentBranch(cwd string) string {
	out, err := RunGit(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		branch := strings.TrimSpace(out)
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}
	return "main"
}

// HasCommits checks if the repository has at least one commit.
func HasCommits(cwd string) bool {
	_, err := RunGit(cwd, "rev-parse", "--verify", "HEAD")
	return err == nil
}

// EnsureInitialCommit creates an empty initial commit if no commits exist.
func EnsureInitialCommit(cwd string) error {
	if !HasCommits(cwd) {
		_, err := RunGit(cwd, "-c", "user.name=teamlead", "-c", "user.email=teamlead@local", "commit", "--allow-empty", "-m", "chore: initial commit by teamlead")
		return err
	}
	return nil
}

// WorktreeInfo holds information about a git worktree.
type WorktreeInfo struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Head     string `json:"head"`
	Bare     bool   `json:"bare"`
	Detached bool   `json:"detached"`
}

// ListWorktrees lists all git worktrees using git worktree list --porcelain.
func ListWorktrees(repoRoot string) ([]WorktreeInfo, error) {
	out, err := RunGit(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo
	hasCurrent := false

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if hasCurrent {
				worktrees = append(worktrees, current)
				current = WorktreeInfo{}
				hasCurrent = false
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = filepath.Clean(strings.TrimPrefix(line, "worktree "))
			hasCurrent = true
		} else if strings.HasPrefix(line, "HEAD ") {
			current.Head = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		} else if line == "bare" {
			current.Bare = true
		} else if line == "detached" {
			current.Detached = true
		}
	}

	if hasCurrent {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// CreateWorktree creates a git worktree at worktreePath checked out to branchName.
func CreateWorktree(repoRoot, worktreePath, branchName, baseBranch string) error {
	if err := EnsureInitialCommit(repoRoot); err != nil {
		return fmt.Errorf("failed to ensure initial commit: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return err
	}

	// Check if branch already exists
	_, err := RunGit(repoRoot, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", branchName))
	branchExists := (err == nil)

	cleanWtPath := filepath.ToSlash(worktreePath)
	if branchExists {
		_, err = RunGit(repoRoot, "worktree", "add", cleanWtPath, branchName)
	} else {
		_, err = RunGit(repoRoot, "worktree", "add", "-b", branchName, cleanWtPath, baseBranch)
	}

	return err
}

// RemoveWorktree deletes a worktree and cleans up git worktree metadata.
func RemoveWorktree(repoRoot, worktreePath string, force bool) error {
	cleanWtPath := filepath.ToSlash(worktreePath)
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, cleanWtPath)

	_, _ = RunGit(repoRoot, args...)
	_, _ = RunGit(repoRoot, "worktree", "prune")

	if fi, err := os.Stat(worktreePath); err == nil && fi.IsDir() {
		_ = filepath.Walk(worktreePath, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0666)
			}
			return nil
		})
		_ = os.RemoveAll(worktreePath)
		_, _ = RunGit(repoRoot, "worktree", "prune")
	}

	return nil
}

// WorktreeStatus contains dirty inspection and branch details for a worktree.
type WorktreeStatus struct {
	Exists         bool     `json:"exists"`
	Branch         string   `json:"branch"`
	LastCommit     string   `json:"last_commit"`
	IsDirty        bool     `json:"is_dirty"`
	ModifiedFiles  []string `json:"modified_files"`
	StagedFiles    []string `json:"staged_files"`
	UntrackedFiles []string `json:"untracked_files"`
	AllDirtyFiles  []string `json:"all_dirty_files"`
}

// GetWorktreeStatus inspects a worktree path for uncommitted changes and last commit.
func GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	fi, err := os.Stat(worktreePath)
	if err != nil || !fi.IsDir() {
		return &WorktreeStatus{Exists: false}, nil
	}

	statusOut, _ := RunGit(worktreePath, "status", "--porcelain")
	var modifiedFiles []string
	var stagedFiles []string
	var untrackedFiles []string

	lines := strings.Split(statusOut, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		indexStatus := line[0]
		worktreeStatus := line[1]
		filePath := strings.TrimSpace(line[3:])
		if strings.Contains(filePath, " -> ") {
			parts := strings.Split(filePath, " -> ")
			if len(parts) == 2 {
				filePath = parts[1]
			}
		}

		cleanFilePath := filepath.ToSlash(filePath)
		// Filter internal teamlead files and OS artifacts
		if strings.HasPrefix(cleanFilePath, ".teamlead/") ||
			cleanFilePath == ".teamlead" ||
			cleanFilePath == "COLLABORATION.md" ||
			cleanFilePath == "AGENTS.md" ||
			cleanFilePath == ".DS_Store" {
			continue
		}

		if indexStatus == 'M' || indexStatus == 'A' || indexStatus == 'D' || indexStatus == 'R' {
			stagedFiles = append(stagedFiles, filePath)
		}
		if worktreeStatus == 'M' || worktreeStatus == 'D' {
			modifiedFiles = append(modifiedFiles, filePath)
		}
		if indexStatus == '?' && worktreeStatus == '?' {
			untrackedFiles = append(untrackedFiles, filePath)
		}
	}

	allDirtyMap := make(map[string]bool)
	for _, f := range modifiedFiles {
		allDirtyMap[f] = true
	}
	for _, f := range stagedFiles {
		allDirtyMap[f] = true
	}
	for _, f := range untrackedFiles {
		allDirtyMap[f] = true
	}

	var allDirty []string
	for f := range allDirtyMap {
		allDirty = append(allDirty, f)
	}
	sort.Strings(allDirty)

	logOut, err := RunGit(worktreePath, "log", "-1", "--pretty=format:%h %s (%cr)")
	lastCommit := "No commits"
	if err == nil && strings.TrimSpace(logOut) != "" {
		lastCommit = strings.TrimSpace(logOut)
	}

	currentBranch := GetCurrentBranch(worktreePath)

	return &WorktreeStatus{
		Exists:         true,
		Branch:         currentBranch,
		LastCommit:     lastCommit,
		IsDirty:        len(allDirty) > 0,
		ModifiedFiles:  modifiedFiles,
		StagedFiles:    stagedFiles,
		UntrackedFiles: untrackedFiles,
		AllDirtyFiles:  allDirty,
	}, nil
}

// GetBranchDivergence returns (behindCount, aheadCount) of branch relative to baseBranch.
func GetBranchDivergence(repoRoot, branch, baseBranch string) (behind, ahead int, err error) {
	out, err := RunGit(repoRoot, "rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", baseBranch, branch))
	if err != nil {
		return 0, 0, nil
	}

	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) == 2 {
		b, err1 := strconv.Atoi(parts[0])
		a, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil {
			return b, a, nil
		}
	}

	return 0, 0, nil
}

// GetCommittedFilesDiff returns files committed on branch that differ from baseBranch.
func GetCommittedFilesDiff(repoRoot, branch, baseBranch string) ([]string, error) {
	out, err := RunGit(repoRoot, "diff", "--name-only", fmt.Sprintf("%s...%s", baseBranch, branch))
	if err != nil {
		return nil, nil
	}

	var files []string
	for _, f := range strings.Split(out, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// SyncWorktreeBranch syncs the worktree's branch with baseBranch using rebase or merge.
func SyncWorktreeBranch(worktreePath, baseBranch, strategy string) (bool, string, error) {
	status, err := GetWorktreeStatus(worktreePath)
	if err != nil {
		return false, "", err
	}

	if status.IsDirty {
		return false, "Cannot sync: worktree has uncommitted changes. Commit or stash them first.", nil
	}

	if strategy == "rebase" {
		_, err := RunGit(worktreePath, "rebase", baseBranch)
		if err != nil {
			_, _ = RunGit(worktreePath, "rebase", "--abort")
			return false, fmt.Sprintf("Rebase conflict against %s:\n%v", baseBranch, err), nil
		}
		return true, fmt.Sprintf("Successfully rebased onto %s.", baseBranch), nil
	}

	// Merge strategy
	_, err = RunGit(worktreePath, "merge", baseBranch, "-m", fmt.Sprintf("Merge %s into branch", baseBranch))
	if err != nil {
		_, _ = RunGit(worktreePath, "merge", "--abort")
		return false, fmt.Sprintf("Merge conflict against %s:\n%v", baseBranch, err), nil
	}
	return true, fmt.Sprintf("Successfully merged %s.", baseBranch), nil
}

// MergeBranchIntoBase merges sourceBranch into baseBranch on the main repository.
func MergeBranchIntoBase(repoRoot, sourceBranch, baseBranch, strategy, commitMessage string) (bool, string, error) {
	currentBranch := GetCurrentBranch(repoRoot)

	mainStatus, err := GetWorktreeStatus(repoRoot)
	if err != nil {
		return false, "", err
	}
	if mainStatus.IsDirty {
		return false, fmt.Sprintf("Main repository at %s has uncommitted changes. Please clean or stash before merging.", repoRoot), nil
	}

	// Checkout base branch
	if _, err := RunGit(repoRoot, "checkout", baseBranch); err != nil {
		return false, fmt.Sprintf("Failed to checkout %s: %v", baseBranch, err), nil
	}

	defer func() {
		if currentBranch != baseBranch {
			_, _ = RunGit(repoRoot, "checkout", currentBranch)
		}
	}()

	msg := commitMessage
	if msg == "" {
		msg = fmt.Sprintf("feat: integrate agent branch %s", sourceBranch)
	}

	switch strategy {
	case "squash":
		if _, err := RunGit(repoRoot, "merge", "--squash", sourceBranch); err != nil {
			_, _ = RunGit(repoRoot, "reset", "--hard", "HEAD")
			return false, fmt.Sprintf("Merge squash conflict: %v", err), nil
		}
		if _, err := RunGit(repoRoot, "-c", "user.name=teamlead", "-c", "user.email=teamlead@local", "commit", "-m", msg); err != nil {
			_, _ = RunGit(repoRoot, "reset", "--hard", "HEAD")
			return false, fmt.Sprintf("Commit squashed changes failed: %v", err), nil
		}
		return true, fmt.Sprintf("Successfully squash-merged %s into %s.", sourceBranch, baseBranch), nil

	case "rebase":
		if _, err := RunGit(repoRoot, "rebase", sourceBranch); err != nil {
			_, _ = RunGit(repoRoot, "rebase", "--abort")
			return false, fmt.Sprintf("Rebase conflict: %v", err), nil
		}
		return true, fmt.Sprintf("Successfully fast-forward/rebased %s into %s.", sourceBranch, baseBranch), nil

	default: // merge commit
		if _, err := RunGit(repoRoot, "merge", "--no-ff", sourceBranch, "-m", msg); err != nil {
			_, _ = RunGit(repoRoot, "merge", "--abort")
			return false, fmt.Sprintf("Merge conflict: %v", err), nil
		}
		return true, fmt.Sprintf("Successfully merged %s into %s with merge commit.", sourceBranch, baseBranch), nil
	}
}

// DeleteBranch deletes a local git branch.
func DeleteBranch(repoRoot, branchName string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := RunGit(repoRoot, "branch", flag, branchName)
	return err
}
