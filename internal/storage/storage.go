package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
)

var (
	lockMu      sync.Mutex
	activeLocks = make(map[string]*lockState)
)

type lockState struct {
	handle osLockHandle
	depth  int
}

// WithFileLock acquires an exclusive advisory file lock on .teamlead/.lock.
// It supports re-entrant locking within the current process to prevent self-deadlocks.
func WithFileLock(repoRoot string, fn func() error) error {
	tlDir, err := config.GetTeamleadDir(repoRoot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(tlDir, 0755); err != nil {
		return err
	}

	lockFilePath := filepath.Join(tlDir, config.GlobalLockFile)

	lockMu.Lock()
	state, exists := activeLocks[lockFilePath]
	if exists {
		state.depth++
		lockMu.Unlock()
		defer func() {
			lockMu.Lock()
			state.depth--
			if state.depth == 0 {
				_ = releaseOSLock(state.handle)
				delete(activeLocks, lockFilePath)
			}
			lockMu.Unlock()
		}()
		return fn()
	}

	handle, err := acquireOSLock(lockFilePath)
	if err != nil {
		lockMu.Unlock()
		return err
	}

	activeLocks[lockFilePath] = &lockState{
		handle: handle,
		depth:  1,
	}
	lockMu.Unlock()

	defer func() {
		lockMu.Lock()
		st, ok := activeLocks[lockFilePath]
		if ok {
			st.depth--
			if st.depth == 0 {
				_ = releaseOSLock(st.handle)
				delete(activeLocks, lockFilePath)
			}
		}
		lockMu.Unlock()
	}()

	return fn()
}

// ReadJSONFile reads and parses a JSON file from .teamlead/.
// If the file does not exist, it returns nil without error.
func ReadJSONFile(repoRoot, filename string, target interface{}) error {
	tlDir, err := config.GetTeamleadDir(repoRoot)
	if err != nil {
		return err
	}

	filePath := filepath.Join(tlDir, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, target)
}

// WriteJSONFile atomically writes data as formatted JSON into .teamlead/.
func WriteJSONFile(repoRoot, filename string, data interface{}) error {
	tlDir, err := config.GetTeamleadDir(repoRoot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(tlDir, 0755); err != nil {
		return err
	}

	targetPath := filepath.Join(tlDir, filename)
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(tlDir, fmt.Sprintf("%s-*.tmp", filename))
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(bytes); err != nil {
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

	var renameErr error
	for i := 0; i < 10; i++ {
		_ = os.Chmod(targetPath, 0666)
		renameErr = os.Rename(tmpPath, targetPath)
		if renameErr == nil {
			return nil
		}
		_ = os.Remove(targetPath)
		renameErr = os.Rename(tmpPath, targetPath)
		if renameErr == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return renameErr
}

// EventEntry represents an audit log event in events.jsonl.
type EventEntry struct {
	Timestamp string                 `json:"timestamp"`
	Event     string                 `json:"event"`
	Agent     *string                `json:"agent"`
	Details   map[string]interface{} `json:"details"`
}

// LogEvent appends a structured event to .teamlead/events.jsonl.
func LogEvent(repoRoot, eventType string, agent *string, details map[string]interface{}) {
	tlDir, err := config.GetTeamleadDir(repoRoot)
	if err != nil {
		return
	}

	_ = os.MkdirAll(tlDir, 0755)
	eventsPath := filepath.Join(tlDir, config.EventsFile)

	if details == nil {
		details = make(map[string]interface{})
	}

	entry := EventEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Event:     eventType,
		Agent:     agent,
		Details:   details,
	}

	bytes, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(append(bytes, '\n'))
}

// ReadRecentEvents reads the last N events from .teamlead/events.jsonl, newest first.
func ReadRecentEvents(repoRoot string, limit int) ([]EventEntry, error) {
	tlDir, err := config.GetTeamleadDir(repoRoot)
	if err != nil {
		return nil, err
	}

	eventsPath := filepath.Join(tlDir, config.EventsFile)
	f, err := os.Open(eventsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		text := scanner.Text()
		if text != "" {
			lines = append(lines, text)
		}
	}

	if limit <= 0 {
		limit = 10
	}

	start := len(lines) - limit
	if start < 0 {
		start = 0
	}

	var results []EventEntry
	for i := len(lines) - 1; i >= start; i-- {
		var entry EventEntry
		if err := json.Unmarshal([]byte(lines[i]), &entry); err == nil {
			results = append(results, entry)
		}
	}

	return results, nil
}
