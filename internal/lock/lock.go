package lock

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/JordyDevrix/teamlead-cli/internal/config"
	"github.com/JordyDevrix/teamlead-cli/internal/storage"
)

// FileLock represents an active lease on a file or glob pattern.
type FileLock struct {
	Path       string  `json:"path"`
	Agent      string  `json:"agent"`
	TaskID     *string `json:"task_id"`
	Reason     *string `json:"reason"`
	AcquiredAt string  `json:"acquired_at"`
	ExpiresAt  string  `json:"expires_at"`
}

// IsExpired checks if the lock has expired based on UTC now.
func (l *FileLock) IsExpired() bool {
	t, err := time.Parse(time.RFC3339, l.ExpiresAt)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(t)
}

// RemainingMinutes returns remaining minutes before expiration.
func (l *FileLock) RemainingMinutes() float64 {
	t, err := time.Parse(time.RFC3339, l.ExpiresAt)
	if err != nil {
		return 0
	}
	diff := time.Until(t)
	mins := diff.Minutes()
	if mins < 0 {
		return 0
	}
	return mins
}

// NormalizePattern cleans path patterns for consistent matching.
func NormalizePattern(pat string) string {
	p := strings.TrimSpace(pat)
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return strings.TrimRight(p, "/")
}

func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i += 2
				continue
			}
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
		i++
	}
	b.WriteString("$")
	return b.String()
}

func fnmatchMatch(pattern, target string) bool {
	reStr := globToRegex(pattern)
	re, err := regexp.Compile(reStr)
	if err != nil {
		return false
	}
	return re.MatchString(target)
}

// PatternsOverlap checks if two file patterns or paths overlap.
func PatternsOverlap(pat1, pat2 string) bool {
	n1 := NormalizePattern(pat1)
	n2 := NormalizePattern(pat2)

	if n1 == n2 {
		return true
	}

	if n1 == "*" || n1 == "**" || n2 == "*" || n2 == "**" {
		return true
	}

	if fnmatchMatch(n1, n2) || fnmatchMatch(n2, n1) {
		return true
	}

	// Check directory prefix containment
	prefix1 := strings.TrimRight(n1, "/*")
	prefix2 := strings.TrimRight(n2, "/*")

	if strings.HasPrefix(n2, prefix1+"/") {
		return true
	}
	if strings.HasPrefix(n1, prefix2+"/") {
		return true
	}

	return false
}

// GetActiveLocks retrieves all active locks from storage.
func GetActiveLocks(repoRoot string, cleanExpired bool) ([]FileLock, error) {
	var locks []FileLock
	if err := storage.ReadJSONFile(repoRoot, config.LocksFile, &locks); err != nil {
		return nil, err
	}

	var active []FileLock
	hasExpired := false

	for _, l := range locks {
		if cleanExpired && l.IsExpired() {
			hasExpired = true
			continue
		}
		active = append(active, l)
	}

	if cleanExpired && hasExpired {
		_ = storage.WithFileLock(repoRoot, func() error {
			return storage.WriteJSONFile(repoRoot, config.LocksFile, active)
		})
	}

	return active, nil
}

// CheckConflicts checks if any paths conflict with locks held by other agents.
func CheckConflicts(repoRoot string, paths []string, agent string) ([]FileLock, error) {
	activeLocks, err := GetActiveLocks(repoRoot, true)
	if err != nil {
		return nil, err
	}

	var conflicts []FileLock
	for _, reqPath := range paths {
		for _, l := range activeLocks {
			if l.Agent != agent && PatternsOverlap(reqPath, l.Path) {
				alreadyAdded := false
				for _, c := range conflicts {
					if c.Path == l.Path && c.Agent == l.Agent {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					conflicts = append(conflicts, l)
				}
			}
		}
	}

	return conflicts, nil
}

// AcquireLocks attempts to acquire exclusive leases on paths for an agent.
func AcquireLocks(repoRoot string, paths []string, agent string, taskID, reason *string, ttlMinutes *int) (bool, []FileLock, []FileLock, error) {
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return false, nil, nil, err
	}

	ttl := cfg.LockTTLMinutes
	if ttlMinutes != nil && *ttlMinutes > 0 {
		ttl = *ttlMinutes
	}

	var acquired []FileLock
	var conflicts []FileLock

	err = storage.WithFileLock(repoRoot, func() error {
		activeLocks, err := GetActiveLocks(repoRoot, true)
		if err != nil {
			return err
		}

		// Check for conflicts
		for _, reqPath := range paths {
			normReq := NormalizePattern(reqPath)
			for _, l := range activeLocks {
				if l.Agent != agent && PatternsOverlap(normReq, l.Path) {
					alreadyAdded := false
					for _, c := range conflicts {
						if c.Path == l.Path && c.Agent == l.Agent {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						conflicts = append(conflicts, l)
					}
				}
			}
		}

		if len(conflicts) > 0 {
			return nil
		}

		now := time.Now().UTC()
		expires := now.Add(time.Duration(ttl) * time.Minute)
		acquiredStr := now.Format(time.RFC3339)
		expiresStr := expires.Format(time.RFC3339)

		// Map existing locks by (path, agent) to refresh or add
		lockMap := make(map[string]FileLock)
		for _, l := range activeLocks {
			key := fmt.Sprintf("%s::%s", NormalizePattern(l.Path), l.Agent)
			lockMap[key] = l
		}

		for _, p := range paths {
			norm := NormalizePattern(p)
			newLock := FileLock{
				Path:       norm,
				Agent:      agent,
				TaskID:     taskID,
				Reason:     reason,
				AcquiredAt: acquiredStr,
				ExpiresAt:  expiresStr,
			}
			key := fmt.Sprintf("%s::%s", norm, agent)
			lockMap[key] = newLock
			acquired = append(acquired, newLock)
		}

		var updated []FileLock
		for _, l := range lockMap {
			updated = append(updated, l)
		}

		if err := storage.WriteJSONFile(repoRoot, config.LocksFile, updated); err != nil {
			return err
		}

		var pathList []string
		for _, l := range acquired {
			pathList = append(pathList, l.Path)
		}

		storage.LogEvent(repoRoot, "LOCK_ACQUIRED", &agent, map[string]interface{}{
			"paths":       pathList,
			"task_id":     taskID,
			"ttl_minutes": ttl,
		})

		return nil
	})

	if err != nil {
		return false, nil, nil, err
	}

	if len(conflicts) > 0 {
		return false, nil, conflicts, nil
	}

	return true, acquired, nil, nil
}

// ReleaseLocks releases specific path locks held by an agent.
func ReleaseLocks(repoRoot string, paths []string, agent string) ([]string, error) {
	normTargets := make(map[string]bool)
	for _, p := range paths {
		normTargets[NormalizePattern(p)] = true
	}

	var released []string

	err := storage.WithFileLock(repoRoot, func() error {
		activeLocks, err := GetActiveLocks(repoRoot, false)
		if err != nil {
			return err
		}

		var remaining []FileLock
		for _, l := range activeLocks {
			normPath := NormalizePattern(l.Path)
			if l.Agent == agent && (normTargets[normPath] || normTargets["*"]) {
				released = append(released, l.Path)
			} else {
				remaining = append(remaining, l)
			}
		}

		if err := storage.WriteJSONFile(repoRoot, config.LocksFile, remaining); err != nil {
			return err
		}

		if len(released) > 0 {
			storage.LogEvent(repoRoot, "LOCK_RELEASED", &agent, map[string]interface{}{
				"paths": released,
			})
		}

		return nil
	})

	return released, err
}

// ReleaseAllForAgent releases all locks held by an agent.
func ReleaseAllForAgent(repoRoot string, agent string) (int, error) {
	count := 0
	err := storage.WithFileLock(repoRoot, func() error {
		activeLocks, err := GetActiveLocks(repoRoot, false)
		if err != nil {
			return err
		}

		var remaining []FileLock
		var releasedPaths []string

		for _, l := range activeLocks {
			if l.Agent == agent {
				count++
				releasedPaths = append(releasedPaths, l.Path)
			} else {
				remaining = append(remaining, l)
			}
		}

		if err := storage.WriteJSONFile(repoRoot, config.LocksFile, remaining); err != nil {
			return err
		}

		if count > 0 {
			storage.LogEvent(repoRoot, "LOCK_RELEASED_ALL", &agent, map[string]interface{}{
				"paths": releasedPaths,
				"count": count,
			})
		}

		return nil
	})

	return count, err
}

// RefreshAgentLocks extends the TTL for all locks held by an agent.
func RefreshAgentLocks(repoRoot string, agent string, ttlMinutes *int) (int, error) {
	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		return 0, err
	}

	ttl := cfg.LockTTLMinutes
	if ttlMinutes != nil && *ttlMinutes > 0 {
		ttl = *ttlMinutes
	}

	count := 0
	err = storage.WithFileLock(repoRoot, func() error {
		activeLocks, err := GetActiveLocks(repoRoot, false)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		newExp := now.Add(time.Duration(ttl) * time.Minute).Format(time.RFC3339)

		for i := range activeLocks {
			if activeLocks[i].Agent == agent {
				activeLocks[i].ExpiresAt = newExp
				count++
			}
		}

		return storage.WriteJSONFile(repoRoot, config.LocksFile, activeLocks)
	})

	return count, err
}
