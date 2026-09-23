package document

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Title\n\nLine 2\nLine 3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.Path != path {
		t.Errorf("expected path %s, got %s", path, doc.Path)
	}
	if doc.Content != content {
		t.Errorf("content mismatch")
	}
	if doc.Hash == "" {
		t.Error("hash should not be empty")
	}
	if doc.LineCount() != 5 { // trailing newline creates empty last line
		t.Errorf("expected 5 lines, got %d", doc.LineCount())
	}
}

func TestLineAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte("line one\nline two\nline three"), 0644); err != nil {
		t.Fatal(err)
	}

	doc, _ := Load(path)

	tests := []struct {
		line int
		want string
	}{
		{1, "line one"},
		{2, "line two"},
		{3, "line three"},
		{0, ""},  // out of range
		{4, ""},  // out of range
		{-1, ""}, // out of range
	}

	for _, tt := range tests {
		got := doc.LineAt(tt.line)
		if got != tt.want {
			t.Errorf("LineAt(%d) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("/nonexistent/file.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
