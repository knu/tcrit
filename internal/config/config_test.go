package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergesGlobalAndProject(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	project := t.TempDir()

	writeConfig(t, filepath.Join(configHome, "tcrit", "config.json"), `{
		"author": "Global Author",
		"quiet": true,
		"cleanup_on_approve": false,
		"ignore_patterns": ["*.lock"],
		"prompts": {"on_finish_approved": "inline:global", "on_finish_unresolved": "inline:global"}
	}`)
	writeConfig(t, ProjectPath(project), `{
		"author": "Project Author",
		"quiet": false,
		"ignore_patterns": ["*.min.js", "*.lock"],
		"prompts": {"on_finish_unresolved": "inline:project"}
	}`)

	cfg, err := Load(project)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}

	if cfg.Author != "Project Author" {
		t.Errorf("Author = %q, want project override", cfg.Author)
	}
	if cfg.Quiet {
		t.Error("project quiet=false should override global true")
	}
	if cfg.CleanupOnApprove {
		t.Error("global cleanup_on_approve=false should override the default true")
	}
	want := []string{"*.lock", "*.min.js"}
	if len(cfg.IgnorePatterns) != len(want) {
		t.Fatalf("IgnorePatterns = %v, want union %v", cfg.IgnorePatterns, want)
	}
	if cfg.Prompts["on_finish_unresolved"] != "inline:project" {
		t.Errorf("project prompt should win, got %q", cfg.Prompts["on_finish_unresolved"])
	}
	if cfg.Prompts["on_finish_approved"] != "inline:global" {
		t.Errorf("global prompt should survive, got %q", cfg.Prompts["on_finish_approved"])
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if !cfg.CleanupOnApprove {
		t.Error("cleanup_on_approve should default to true")
	}
}
