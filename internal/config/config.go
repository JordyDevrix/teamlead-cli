package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	TeamleadDirName    = ".teamlead"
	ConfigFile         = "config.json"
	TasksFile          = "tasks.json"
	LocksFile          = "locks.json"
	AgentsFile         = "agents.json"
	EventsFile         = "events.jsonl"
	GlobalLockFile     = ".lock"
)

// Config represents project-level teamlead configuration.
type Config struct {
	ProjectName    string  `json:"project_name"`
	BaseBranch     string  `json:"base_branch"`
	WorktreeDir    string  `json:"worktree_dir"`
	MergeStrategy  string  `json:"merge_strategy"`
	TestCommand    *string `json:"test_command"`
	LockTTLMinutes int     `json:"lock_ttl_minutes"`
	AutoSync       bool    `json:"auto_sync"`
}

// DefaultConfig returns default configuration values.
func DefaultConfig() *Config {
	return &Config{
		ProjectName:    "project",
		BaseBranch:     "main",
		WorktreeDir:    filepath.Join(TeamleadDirName, "worktrees"),
		MergeStrategy:  "squash",
		TestCommand:    nil,
		LockTTLMinutes: 60,
		AutoSync:       true,
	}
}

// FindRepoRoot searches upwards from startPath for a .git or .teamlead directory.
func FindRepoRoot(startPath string) (string, error) {
	if startPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		startPath = cwd
	}

	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	current := absPath
	for {
		gitDir := filepath.Join(current, ".git")
		tlDir := filepath.Join(current, TeamleadDirName)

		if fi, err := os.Stat(gitDir); err == nil {
			if fi.IsDir() || !fi.IsDir() { // .git can be file in submodules/worktrees
				return current, nil
			}
		}
		if fi, err := os.Stat(tlDir); err == nil && fi.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return "", errors.New("could not locate git repository root (no .git or .teamlead directory found)")
}

// GetTeamleadDir returns the path to the .teamlead directory.
func GetTeamleadDir(repoRoot string) (string, error) {
	root := repoRoot
	if root == "" {
		var err error
		root, err = FindRepoRoot("")
		if err != nil {
			return "", fmt.Errorf("could not locate git repository root. Please run 'teamlead init' or navigate inside a git repo: %w", err)
		}
	}
	return filepath.Join(root, TeamleadDirName), nil
}

// IsInitialized checks if teamlead has been initialized in repoRoot.
func IsInitialized(repoRoot string) bool {
	root := repoRoot
	if root == "" {
		var err error
		root, err = FindRepoRoot("")
		if err != nil {
			return false
		}
	}
	cfgPath := filepath.Join(root, TeamleadDirName, ConfigFile)
	_, err := os.Stat(cfgPath)
	return err == nil
}

// LoadConfig loads configuration from .teamlead/config.json.
func LoadConfig(repoRoot string) (*Config, error) {
	tlDir, err := GetTeamleadDir(repoRoot)
	if err != nil {
		return DefaultConfig(), nil
	}

	cfgPath := filepath.Join(tlDir, ConfigFile)
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return DefaultConfig(), nil
	}

	return cfg, nil
}

// SaveConfig writes the configuration to .teamlead/config.json atomically.
func SaveConfig(cfg *Config, repoRoot string) error {
	tlDir, err := GetTeamleadDir(repoRoot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(tlDir, 0755); err != nil {
		return err
	}

	cfgPath := filepath.Join(tlDir, ConfigFile)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(tlDir, "config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, cfgPath)
}
