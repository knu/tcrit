package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func integrationContent(t *testing.T, target, dest string) string {
	t.Helper()
	for _, f := range integrations()[target] {
		if f.dest == dest {
			data, err := f.content()
			if err != nil {
				t.Fatalf("%s %s: %v", target, dest, err)
			}
			return string(data)
		}
	}
	t.Fatalf("%s has no integration file %s", target, dest)
	return ""
}

func destinations(t *testing.T, target string) []string {
	t.Helper()
	var dests []string
	for _, f := range integrations()[target] {
		if target != "opencode" && f.dest != f.globalDest {
			t.Errorf("%s %s: global destination %q differs", target, f.dest, f.globalDest)
		}
		dests = append(dests, f.dest)
	}
	return dests
}

func TestOpenCodeGlobalFilesUseXDGConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	want := map[string]string{
		filepath.Join(".opencode", "commands", "tcrit.md"):            filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "commands", "tcrit.md"),
		filepath.Join(".opencode", "skills", "tcrit-cli", "SKILL.md"): filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "opencode", "skills", "tcrit-cli", "SKILL.md"),
	}
	for _, f := range integrations()["opencode"] {
		if f.globalDest != want[f.dest] {
			t.Errorf("%s global destination = %q, want %q", f.dest, f.globalDest, want[f.dest])
		}
		global, err := integrationDest(f, true)
		if err != nil || global != want[f.dest] {
			t.Errorf("integrationDest(%s, global) = %q, %v", f.dest, global, err)
		}
	}
}

func TestIntegrationLayouts(t *testing.T) {
	want := map[string][]string{
		"claude-code": {
			filepath.Join(".claude", "skills", "tcrit", "SKILL.md"),
			filepath.Join(".claude", "skills", "tcrit-cli", "SKILL.md"),
		},
		"codex": {
			filepath.Join(".agents", "skills", "tcrit", "SKILL.md"),
			filepath.Join(".agents", "skills", "tcrit-cli", "SKILL.md"),
		},
		"opencode": {
			filepath.Join(".opencode", "commands", "tcrit.md"),
			filepath.Join(".opencode", "skills", "tcrit-cli", "SKILL.md"),
		},
		"gemini": {
			filepath.Join(".gemini", "agents", "tcrit.md"),
			filepath.Join(".gemini", "skills", "tcrit-cli", "SKILL.md"),
		},
	}
	for target, dests := range want {
		if got := destinations(t, target); !slices.Equal(got, dests) {
			t.Errorf("%s destinations = %q, want %q", target, got, dests)
		}
	}
}

func TestAllIncludesEveryAgent(t *testing.T) {
	names, err := integrationTargets("all", integrations())
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude-code", "codex", "opencode", "gemini"} {
		if !slices.Contains(names, agent) {
			t.Errorf("all target %q lacks %s", names, agent)
		}
	}
	if slices.Contains(names, "prompts") {
		t.Errorf("all target %q must not install prompt templates", names)
	}
}

func TestClaudeSkillsAreVerbatim(t *testing.T) {
	for _, name := range skillNames {
		embedded, err := skillContent.ReadFile("skill/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(".claude", "skills", name, "SKILL.md")
		if got := integrationContent(t, "claude-code", dest); got != string(embedded) {
			t.Errorf("%s is rewritten for Claude Code", dest)
		}
	}
}

func TestCodexSkillsUseCodexConventions(t *testing.T) {
	skill := integrationContent(t, "codex", filepath.Join(".agents", "skills", "tcrit", "SKILL.md"))
	cli := integrationContent(t, "codex", filepath.Join(".agents", "skills", "tcrit-cli", "SKILL.md"))

	for _, unwanted := range []string{"`/tcrit`", "`/tcrit-cli`", "/tcrit ", "Claude Code", "$ARGUMENTS", "allowed-tools:", "argument-hint:", "user-invocable:"} {
		for _, content := range []string{skill, cli} {
			if strings.Contains(content, unwanted) {
				t.Errorf("Codex skill still contains %q", unwanted)
			}
		}
	}
	for _, wanted := range []string{"`$tcrit-cli`", "'Codex'", "tcrit <arguments>"} {
		if !strings.Contains(skill, wanted) {
			t.Errorf("Codex tcrit skill lacks %q", wanted)
		}
	}
	if !strings.Contains(skill, "tcrit --staged") {
		t.Error("Codex tcrit skill does not document staged-only review")
	}
	if !strings.Contains(cli, "`$tcrit`") {
		t.Error("Codex tcrit-cli skill does not point at $tcrit")
	}
	if !strings.HasPrefix(cli, "---\nname: tcrit-cli\ndescription: ") {
		t.Errorf("Codex tcrit-cli frontmatter is malformed:\n%s", cli[:min(len(cli), 200)])
	}
}

func TestOpenCodeCommandKeepsArgumentsAndDropsClaudeKeys(t *testing.T) {
	command := integrationContent(t, "opencode", filepath.Join(".opencode", "commands", "tcrit.md"))
	cli := integrationContent(t, "opencode", filepath.Join(".opencode", "skills", "tcrit-cli", "SKILL.md"))

	if !strings.Contains(command, "tcrit $ARGUMENTS") {
		t.Error("OpenCode command lost $ARGUMENTS")
	}
	if !strings.Contains(command, "tcrit --staged") {
		t.Error("OpenCode command does not document staged-only review")
	}
	if !strings.HasPrefix(command, "---\ndescription: ") {
		t.Errorf("OpenCode command frontmatter should start with description:\n%s", command[:min(len(command), 120)])
	}
	if !strings.Contains(command, "`tcrit-cli`") || strings.Contains(command, "`/tcrit-cli`") {
		t.Error("OpenCode command must reference the tcrit-cli skill by bare name")
	}
	if !strings.Contains(cli, "`/tcrit`") {
		t.Error("OpenCode tcrit-cli must point at the /tcrit command")
	}
	for _, content := range []string{command, cli} {
		for _, unwanted := range []string{"Claude Code", "allowed-tools:", "argument-hint:", "user-invocable:"} {
			if strings.Contains(content, unwanted) {
				t.Errorf("OpenCode file still contains %q", unwanted)
			}
		}
		if !strings.Contains(content, "'OpenCode'") {
			t.Error("OpenCode file is not attributed to OpenCode")
		}
	}
}

func TestGeminiIntegration(t *testing.T) {
	agent := integrationContent(t, "gemini", filepath.Join(".gemini", "agents", "tcrit.md"))
	cli := integrationContent(t, "gemini", filepath.Join(".gemini", "skills", "tcrit-cli", "SKILL.md"))

	if !strings.HasPrefix(agent, "---\nname: tcrit\n") {
		t.Error("Gemini agent frontmatter is malformed")
	}
	if !strings.Contains(agent, "--author 'Gemini'") {
		t.Error("Gemini agent is not attributed to Gemini")
	}
	if !strings.Contains(agent, "tcrit --staged") {
		t.Error("Gemini agent does not document staged-only review")
	}
	if !strings.Contains(cli, "'Gemini'") || strings.Contains(cli, "user-invocable:") {
		t.Error("Gemini tcrit-cli skill is not rewritten for Gemini")
	}
}

func TestDropFrontmatterKeys(t *testing.T) {
	in := []byte("---\nname: x\nallowed-tools: Bash(x *)\ndescription: d\n---\n\nbody: allowed-tools: keep\n")
	got := dropFrontmatterKeys(in, []string{"allowed-tools"})
	want := []byte("---\nname: x\ndescription: d\n---\n\nbody: allowed-tools: keep\n")
	if !bytes.Equal(got, want) {
		t.Errorf("dropFrontmatterKeys = %q, want %q", got, want)
	}
	if got := dropFrontmatterKeys([]byte("no frontmatter\n"), []string{"name"}); string(got) != "no frontmatter\n" {
		t.Errorf("content without frontmatter was altered: %q", got)
	}
}

func TestPluginSkillsMatchEmbeddedSources(t *testing.T) {
	for _, name := range skillNames {
		embedded, err := skillContent.ReadFile("skill/" + name + "/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		pluginPath := filepath.Join("..", "..", "plugin", "tcrit", "skills", name, "SKILL.md")
		plugin, err := os.ReadFile(pluginPath)
		if err != nil {
			t.Fatalf("reading %s: %v", pluginPath, err)
		}
		if !bytes.Equal(plugin, embedded) {
			t.Errorf("%s differs from the embedded skill; copy internal/cli/skill/%s/SKILL.md over it", pluginPath, name)
		}
	}
}
