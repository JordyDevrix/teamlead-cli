package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got '%s'", cfg.BaseBranch)
	}
	if cfg.MergeStrategy != "squash" {
		t.Errorf("expected MergeStrategy 'squash', got '%s'", cfg.MergeStrategy)
	}
	if cfg.LockTTLMinutes != 60 {
		t.Errorf("expected LockTTLMinutes 60, got %d", cfg.LockTTLMinutes)
	}
	if !cfg.AutoSync {
		t.Errorf("expected AutoSync true, got %v", cfg.AutoSync)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := DefaultConfig()
	cfg.ProjectName = "custom-project"
	cfg.BaseBranch = "develop"
	cfg.MergeStrategy = "rebase"
	cmd := "npm test"
	cfg.TestCommand = &cmd

	if err := SaveConfig(cfg, tmpDir); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.ProjectName != "custom-project" {
		t.Errorf("expected 'custom-project', got '%s'", loaded.ProjectName)
	}
	if loaded.BaseBranch != "develop" {
		t.Errorf("expected 'develop', got '%s'", loaded.BaseBranch)
	}
	if loaded.MergeStrategy != "rebase" {
		t.Errorf("expected 'rebase', got '%s'", loaded.MergeStrategy)
	}
	if loaded.TestCommand == nil || *loaded.TestCommand != "npm test" {
		t.Errorf("expected test command 'npm test', got %v", loaded.TestCommand)
	}
}

func TestFindRepoRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a nested directory
	subDir := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create .git at tmpDir
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	root, err := FindRepoRoot(subDir)
	if err != nil {
		t.Fatalf("FindRepoRoot failed: %v", err)
	}

	evalTmp, _ := filepath.EvalSymlinks(tmpDir)
	evalRoot, _ := filepath.EvalSymlinks(root)
	if evalRoot != evalTmp {
		t.Errorf("expected root '%s', got '%s'", evalTmp, evalRoot)
	}
}
