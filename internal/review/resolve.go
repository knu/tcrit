package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sessionKeyRe = regexp.MustCompile(`^[0-9a-f]{12}$`)

// ValidSessionKey reports whether s looks like a session key.
func ValidSessionKey(s string) bool {
	return sessionKeyRe.MatchString(s)
}

// ReadSessionEntry loads one registry entry.
func ReadSessionEntry(key string) (*SessionEntry, error) {
	data, err := os.ReadFile(sessionEntryPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no review session %s", key)
		}
		return nil, fmt.Errorf("reading session entry: %w", err)
	}
	var e SessionEntry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parsing session entry: %w", err)
	}
	return &e, nil
}

// OpenSessionFromEntry opens the review folder a registry entry points at.
func OpenSessionFromEntry(e SessionEntry) (*Session, error) {
	dir := Dir("", e.Key)
	if e.ReviewPath != "" {
		dir = filepath.Dir(e.ReviewPath)
	}
	s, err := openSessionAt(e.Key, dir)
	if err != nil {
		return nil, err
	}
	s.Meta = e
	return s, nil
}

// ResolveTarget finds the review session a comment command should operate
// on: an explicit --session key, the single session registered for the
// current directory, or (when none is registered) the git-mode session for
// the current branch.
func ResolveTarget(dataRoot, sessionID string) (*Session, error) {
	if sessionID != "" {
		if !ValidSessionKey(sessionID) {
			return nil, fmt.Errorf("invalid session ID %q", sessionID)
		}
		e, err := ReadSessionEntry(sessionID)
		if err != nil {
			return nil, err
		}
		return OpenSessionFromEntry(*e)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving cwd: %w", err)
	}
	entries, err := ListSessionEntries()
	if err != nil {
		return nil, err
	}
	var matches []SessionEntry
	for _, e := range entries {
		if e.CWD == cwd {
			matches = append(matches, e)
		}
	}
	switch len(matches) {
	case 1:
		return OpenSessionFromEntry(matches[0])
	case 0:
		return OpenCodeSession(dataRoot)
	default:
		var keys []string
		for _, e := range matches {
			label := e.Key
			if len(e.Args) > 0 {
				label += " (" + strings.Join(e.Args, " ") + ")"
			} else if e.Branch != "" {
				label += " (" + e.Branch + ")"
			}
			keys = append(keys, label)
		}
		return nil, fmt.Errorf("multiple review sessions for this directory; use --session <id>:\n  %s",
			strings.Join(keys, "\n  "))
	}
}

// OpenSessionAt loads a review folder at an explicit directory.
func OpenSessionAt(key, dir string) (*Session, error) {
	return openSessionAt(key, dir)
}

// openSessionAt loads a review folder at an explicit directory.
func openSessionAt(key, dir string) (*Session, error) {
	s := &Session{Key: key, Dir: dir}
	data, err := os.ReadFile(JSONPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			s.CJ = NewCritJSON()
			return s, nil
		}
		return nil, fmt.Errorf("reading review: %w", err)
	}
	if err := json.Unmarshal(data, &s.CJ); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", JSONPath(dir), err)
	}
	if s.CJ.Files == nil {
		s.CJ.Files = map[string]CritJSONFile{}
	}
	if s.CJ.ReviewRound == 0 {
		s.CJ.ReviewRound = 1
	}
	return s, nil
}

// FindSessionsByCommentID scans every registered session for a comment ID,
// skipping the session with excludeKey.  Used to redirect replies whose
// target lives in another review file.
func FindSessionsByCommentID(id, excludeKey string) ([]*Session, error) {
	entries, err := ListSessionEntries()
	if err != nil {
		return nil, err
	}
	var found []*Session
	for _, e := range entries {
		if e.Key == excludeKey {
			continue
		}
		s, err := OpenSessionFromEntry(e)
		if err != nil {
			continue
		}
		if s.CJ.ContainsCommentID(id) {
			found = append(found, s)
		}
	}
	return found, nil
}
