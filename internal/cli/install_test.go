package cli

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCodexIntegration(t *testing.T) {
	files := integrations()["codex"]
	wantNames := []string{"tcrit-review", "tcrit-plan-review", "tcrit-code-review"}
	if len(files) != len(wantNames) {
		t.Fatalf("Codex integration has %d files, want %d", len(files), len(wantNames))
	}

	for i, name := range wantNames {
		wantDest := filepath.Join(".agents", "skills", name, "SKILL.md")
		if files[i].dest != wantDest || files[i].globalDest != wantDest {
			t.Errorf("%s destinations = %q, %q; want %q", name, files[i].dest, files[i].globalDest, wantDest)
		}

		data, err := files[i].content()
		if err != nil {
			t.Fatalf("read %s content: %v", name, err)
		}
		content := string(data)
		if !strings.Contains(content, "name: "+name) {
			t.Errorf("%s content has the wrong skill name", name)
		}
		if strings.Contains(content, "/tcrit-") {
			t.Errorf("%s content uses Claude Code skill invocation", name)
		}
		if strings.Contains(content, "'Claude Code'") {
			t.Errorf("%s content attributes replies to Claude Code", name)
		}
	}

	review, err := files[0].content()
	if err != nil {
		t.Fatal(err)
	}
	for _, invocation := range []string{"$tcrit-code-review", "$tcrit-plan-review"} {
		if !strings.Contains(string(review), invocation) {
			t.Errorf("Codex review skill does not invoke %s", invocation)
		}
	}

	for _, i := range []int{1, 2} {
		data, err := files[i].content()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "--author 'Codex'") {
			t.Errorf("%s does not attribute replies to Codex", wantNames[i])
		}
	}
}

func TestAllIntegrationTargetsIncludeCodex(t *testing.T) {
	targets, err := integrationTargets("all", integrations())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude-code", "codex", "gemini"} {
		if !slices.Contains(targets, target) {
			t.Errorf("all targets = %v; missing %s", targets, target)
		}
	}
}
