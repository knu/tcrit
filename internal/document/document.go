package document

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

type Document struct {
	Path    string
	Content string
	Lines   []string
	Hash    string
}

func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading document: %w", err)
	}

	return FromContent(path, data), nil
}

// FromContent builds a document from content supplied by another source.
func FromContent(path string, data []byte) *Document {
	content := string(data)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	lines := strings.Split(content, "\n")

	return &Document{
		Path:    path,
		Content: content,
		Lines:   lines,
		Hash:    hash,
	}
}

func (d *Document) LineAt(n int) string {
	if n < 1 || n > len(d.Lines) {
		return ""
	}
	return d.Lines[n-1]
}

func (d *Document) LineCount() int {
	return len(d.Lines)
}

func HashContent(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}
