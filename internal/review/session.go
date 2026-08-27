package review

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevindutra/crit/internal/document"
	"gopkg.in/yaml.v3"
)

const sessionFile = ".crit/code-review.yaml"

// CodeReviewSession tracks which files belong to the current code review.
type CodeReviewSession struct {
	Files     []string  `yaml:"files"`
	DiffBase  string    `yaml:"diff_base"`
	CreatedAt time.Time `yaml:"created_at"`
}

// SaveSession writes the session manifest.
func SaveSession(session *CodeReviewSession) error {
	if err := document.EnsureDirs(); err != nil {
		return err
	}

	data, err := yaml.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	dir := filepath.Dir(sessionFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}

	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// LoadSession reads the current code review session.
func LoadSession() (*CodeReviewSession, error) {
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no active code review session (run `tcrit review --code` first)")
		}
		return nil, fmt.Errorf("reading session: %w", err)
	}

	var session CodeReviewSession
	if err := yaml.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session: %w", err)
	}

	return &session, nil
}

// CodeFileStatus represents a single file's review status in aggregate output.
type CodeFileStatus struct {
	File     string    `json:"file"`
	Comments []Comment `json:"comments"`
}

// CodeReviewStatus is the aggregate status for all files in a code review.
type CodeReviewStatus struct {
	Files         []CodeFileStatus `json:"files"`
	TotalComments int              `json:"total_comments"`
}

// AggregateStatus loads ReviewState for all files in the current session.
func AggregateStatus() (*CodeReviewStatus, error) {
	session, err := LoadSession()
	if err != nil {
		return nil, err
	}

	result := &CodeReviewStatus{}
	for _, file := range session.Files {
		state, err := Load(file)
		if err != nil {
			// Skip files that can't be loaded
			continue
		}
		result.Files = append(result.Files, CodeFileStatus{
			File:     file,
			Comments: state.Comments,
		})
		result.TotalComments += len(state.Comments)
	}

	return result, nil
}
