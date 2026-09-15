package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JordyDevrix/teamlead-cli/internal/git"
)

func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return buf.String(), err
}

func cleanTempDir(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			_ = os.Chmod(path, 0666)
		}
		return nil
	})
	_ = os.RemoveAll(dir)
}

func TestCLIInit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-cli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origWd)
		cleanTempDir(tmpDir)
	}()
	_ = os.Chdir(tmpDir)

	_, err = executeCommand("init")
	if err != nil {
		t.Fatalf("teamlead init failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, ".teamlead")); err != nil {
		t.Errorf(".teamlead directory was not created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md was not created")
	}
}

func TestCLITaskAndLockFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tl-cli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer func() {
		_ = os.Chdir(origWd)
		cleanTempDir(tmpDir)
	}()
	_ = os.Chdir(tmpDir)

	_, _ = git.RunGit(tmpDir, "init", "-b", "main")
	_, _ = git.RunGit(tmpDir, "config", "user.name", "Test Runner")
	_, _ = git.RunGit(tmpDir, "config", "user.email", "runner@teamlead.ai")
	_ = git.EnsureInitialCommit(tmpDir)

	_, err = executeCommand("init")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Add task
	_, err = executeCommand("task", "add", "Build authentication", "--scope", "src/auth/**")
	if err != nil {
		t.Fatalf("task add failed: %v", err)
	}

	// Check lock before claiming
	_, err = executeCommand("lock", "check", "src/auth/login.go")
	if err != nil {
		t.Errorf("expected lock check to be free, got err: %v", err)
	}

	// Claim task
	_, err = executeCommand("task", "claim", "T-1", "--agent", "claude")
	if err != nil {
		t.Fatalf("task claim failed: %v", err)
	}

	// Now check lock for another agent -> should error
	_, err = executeCommand("lock", "check", "src/auth/login.go", "--agent", "aider")
	if err == nil {
		t.Error("expected lock check to report conflict for aider")
	}

	// Status dashboard
	_, err = executeCommand("status")
	if err != nil {
		t.Errorf("status dashboard failed: %v", err)
	}
}

func TestCLIGuide(t *testing.T) {
	out, err := executeCommand("guide", "--format", "claude")
	if err != nil {
		t.Fatalf("guide failed: %v", err)
	}
	if !strings.Contains(out, "TeamLead Autonomous Multi-Agent Protocol") {
		t.Errorf("expected protocol text in output")
	}
}

func TestCLIVersion(t *testing.T) {
	out, err := executeCommand("--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(out, Version) {
		t.Errorf("expected version %q in output, got: %s", Version, out)
	}
}
