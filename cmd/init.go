package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/context"
	"github.com/JordyDevrix/teamlead-cli/internal/git"
	"github.com/JordyDevrix/teamlead-cli/internal/ui"
)

var (
	initBaseBranch string
	initStrategy   string
	initTestCmd    string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize teamlead coordination in the current repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// 1. Git initialization check
		if !git.IsGitRepo(cwd) {
			ui.Info("No git repository detected. Initializing git repository...")
			if _, err := git.RunGit(cwd, "init"); err != nil {
				return fmt.Errorf("git init failed: %w", err)
			}
		}

		repoRoot, err := config.FindRepoRoot(cwd)
		if err != nil {
			repoRoot = cwd
		}

		// 2. Ensure initial commit so git worktrees can be created
		if err := git.EnsureInitialCommit(repoRoot); err != nil {
			return fmt.Errorf("failed to ensure initial commit: %w", err)
		}

		currentBranch := initBaseBranch
		if currentBranch == "" {
			currentBranch = git.GetCurrentBranch(repoRoot)
		}

		tlDir := filepath.Join(repoRoot, config.TeamleadDirName)
		if err := os.MkdirAll(tlDir, 0755); err != nil {
			return err
		}

		cfg := config.DefaultConfig()
		cfg.ProjectName = filepath.Base(repoRoot)
		cfg.BaseBranch = currentBranch
		cfg.MergeStrategy = initStrategy
		if strings.TrimSpace(initTestCmd) != "" {
			t := strings.TrimSpace(initTestCmd)
			cfg.TestCommand = &t
		}

		if err := config.SaveConfig(cfg, repoRoot); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		// 3. Update .gitignore
		gitignorePath := filepath.Join(repoRoot, ".gitignore")
		ignoreEntries := []string{
			fmt.Sprintf("%s/worktrees/", config.TeamleadDirName),
			fmt.Sprintf("%s/.lock", config.TeamleadDirName),
			fmt.Sprintf("%s/worktrees/**", config.TeamleadDirName),
			"COLLABORATION.md",
		}

		existingIgnores := ""
		if data, err := os.ReadFile(gitignorePath); err == nil {
			existingIgnores = string(data)
		}

		var newIgnores []string
		for _, entry := range ignoreEntries {
			if !strings.Contains(existingIgnores, entry) {
				newIgnores = append(newIgnores, entry)
			}
		}

		if len(newIgnores) > 0 {
			f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				if len(existingIgnores) > 0 && !strings.HasSuffix(existingIgnores, "\n") {
					_, _ = f.WriteString("\n")
				}
				_, _ = f.WriteString("# teamlead workspace isolation\n")
				for _, entry := range newIgnores {
					_, _ = f.WriteString(entry + "\n")
				}
				_ = f.Close()
			}
		}

		// 4. Generate root AGENTS.md
		if _, err := context.UpdateRootAgentsMD(repoRoot); err != nil {
			ui.Warningf("Failed to generate AGENTS.md: %v", err)
		}

		// 5. Commit initial configuration
		_, _ = git.RunGit(repoRoot, "add", ".gitignore", "AGENTS.md")
		_, _ = git.RunGit(repoRoot, "commit", "-m", "chore: initialize teamlead coordination")

		ui.Successf("Initialized teamlead in %s", repoRoot)
		ui.Infof("Base branch set to '%s' with '%s' merge strategy.", currentBranch, initStrategy)
		ui.Info("Generated AGENTS.md instructions for AI models and CLI tools.")
		ui.Info("Run 'teamlead status' anytime to view active agents and locks.")

		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initBaseBranch, "base-branch", "", "Base branch (e.g. main, master)")
	initCmd.Flags().StringVar(&initStrategy, "strategy", "squash", "Default merge strategy (squash, rebase, merge)")
	initCmd.Flags().StringVar(&initTestCmd, "test-cmd", "", "Validation test command before merging (e.g. 'pytest')")
	rootCmd.AddCommand(initCmd)
}
