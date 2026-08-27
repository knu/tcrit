package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunClearAll(t *testing.T) {
	t.Chdir(t.TempDir())
	reviewsDir := filepath.Join(".crit", "reviews")
	if err := os.MkdirAll(reviewsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(reviewsDir, "abc123.yaml"):   "comments: []\n",
		filepath.Join(reviewsDir, "def456.json"):   "{}\n",
		filepath.Join(".crit", "code-review.yaml"): "files: []\n",
		filepath.Join(".crit", ".gitignore"):       "*\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runClearAll(); err != nil {
		t.Fatalf("runClearAll: %v", err)
	}

	for _, path := range []string{
		filepath.Join(reviewsDir, "abc123.yaml"),
		filepath.Join(reviewsDir, "def456.json"),
		filepath.Join(".crit", "code-review.yaml"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s removed", path)
		}
	}
	if _, err := os.Stat(filepath.Join(".crit", ".gitignore")); err != nil {
		t.Errorf("expected .crit/.gitignore preserved: %v", err)
	}
}

func TestRunClearAllWithoutState(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runClearAll(); err != nil {
		t.Fatalf("runClearAll on empty dir: %v", err)
	}
}
