package cli

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/prompt"
	"github.com/knu/tcrit/internal/xdg"
)

//go:embed skill/tcrit-review/SKILL.md skill/tcrit-plan-review/SKILL.md skill/tcrit-code-review/SKILL.md
var skillContent embed.FS

//go:embed agent/gemini/tcrit.md
var geminiContent embed.FS

var installForce bool

// integrationFile is one installable asset with its project- and
// global-relative destinations.
type integrationFile struct {
	content    func() ([]byte, error)
	dest       string // relative to the project root (cwd)
	globalDest string // relative to $HOME, or absolute when starting with "/"
}

func embeddedFile(fs embed.FS, path string) func() ([]byte, error) {
	return func() ([]byte, error) { return fs.ReadFile(path) }
}

func codexSkill(name string) func() ([]byte, error) {
	return func() ([]byte, error) {
		data, err := skillContent.ReadFile("skill/" + name + "/SKILL.md")
		if err != nil {
			return nil, err
		}
		data = bytes.ReplaceAll(data, []byte("/tcrit-"), []byte("$tcrit-"))
		data = bytes.ReplaceAll(data, []byte("'Claude Code'"), []byte("'Codex'"))
		return data, nil
	}
}

// integrations maps install target names to their files, following crit's
// naming (the Claude Code agent is "claude-code").
func integrations() map[string][]integrationFile {
	m := map[string][]integrationFile{}

	var claude []integrationFile
	for _, name := range []string{"tcrit-review", "tcrit-plan-review", "tcrit-code-review"} {
		rel := filepath.Join(".claude", "skills", name, "SKILL.md")
		claude = append(claude, integrationFile{
			content:    embeddedFile(skillContent, "skill/"+name+"/SKILL.md"),
			dest:       rel,
			globalDest: rel,
		})
	}
	m["claude-code"] = claude

	var codex []integrationFile
	for _, name := range []string{"tcrit-review", "tcrit-plan-review", "tcrit-code-review"} {
		rel := filepath.Join(".agents", "skills", name, "SKILL.md")
		codex = append(codex, integrationFile{
			content:    codexSkill(name),
			dest:       rel,
			globalDest: rel,
		})
	}
	m["codex"] = codex

	m["gemini"] = []integrationFile{{
		content:    embeddedFile(geminiContent, "agent/gemini/tcrit.md"),
		dest:       filepath.Join(".gemini", "agents", "tcrit.md"),
		globalDest: filepath.Join(".gemini", "agents", "tcrit.md"),
	}}

	var prompts []integrationFile
	stock := prompt.StockTemplates()
	names := make([]string, 0, len(stock))
	for name := range stock {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := stock[name]
		prompts = append(prompts, integrationFile{
			content:    func() ([]byte, error) { return data, nil },
			dest:       filepath.Join(".tcrit", "prompts", name),
			globalDest: filepath.Join(xdg.ConfigHome(), "prompts", name), // absolute
		})
	}
	m["prompts"] = prompts

	return m
}

func integrationTargets(target string, reg map[string][]integrationFile) ([]string, error) {
	if target == "all" {
		return []string{"claude-code", "codex", "gemini"}, nil
	}
	if _, ok := reg[target]; ok {
		return []string{target}, nil
	}

	available := make([]string, 0, len(reg)+1)
	for name := range reg {
		available = append(available, name)
	}
	sort.Strings(available)
	return nil, fmt.Errorf("unknown target %q (available: %s, all)", target, strings.Join(available, ", "))
}

var installCmd = &cobra.Command{
	Use:   "install [--force] <claude-code|codex|gemini|prompts|all>",
	Short: "Install agent integrations or stock prompt templates",
	Long: `Install agent integrations (skills) or the stock finish prompt
templates.

Run from your home directory to install globally, or from a repository
root to install for that project only, following crit's convention.

Targets:
  claude-code  Claude Code skills (tcrit-review, tcrit-plan-review, tcrit-code-review)
  codex        Codex skills (tcrit-review, tcrit-plan-review, tcrit-code-review)
  gemini       Gemini CLI agent (@tcrit)
  prompts      Stock finish prompt templates (customize after copying)
  all          claude-code + codex + gemini`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		reg := integrations()
		names, err := integrationTargets(target, reg)
		if err != nil {
			return err
		}

		global, err := isGlobalInstall()
		if err != nil {
			return err
		}
		for _, name := range names {
			for _, f := range reg[name] {
				if err := installIntegrationFile(f, global); err != nil {
					return err
				}
			}
		}
		return nil
	},
}

// isGlobalInstall reports whether the install targets global locations:
// crit's rule is simply "run from $HOME".
func isGlobalInstall() (bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	return cwd == home, nil
}

func integrationDest(f integrationFile, global bool) (string, error) {
	if !global {
		return f.dest, nil
	}
	if filepath.IsAbs(f.globalDest) {
		return f.globalDest, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, f.globalDest), nil
}

func installIntegrationFile(f integrationFile, global bool) error {
	dest, err := integrationDest(f, global)
	if err != nil {
		return err
	}
	data, err := f.content()
	if err != nil {
		return err
	}

	if existing, err := os.ReadFile(dest); err == nil {
		if bytes.Equal(existing, data) {
			fmt.Printf("Up to date: %s\n", dest)
			return nil
		}
		if !installForce {
			fmt.Printf("Skipping %s (exists with different content, use --force to overwrite)\n", dest)
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	fmt.Printf("Installed %s\n", dest)
	return nil
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installed integrations for missing or stale files",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := integrations()
		names := make([]string, 0, len(reg))
		for name := range reg {
			names = append(names, name)
		}
		sort.Strings(names)

		stale := 0
		for _, name := range names {
			// Prompt templates are meant to be customized after copying,
			// so differing content is not staleness.
			if name == "prompts" {
				continue
			}
			for _, f := range reg[name] {
				for _, global := range []bool{false, true} {
					dest, err := integrationDest(f, global)
					if err != nil {
						continue
					}
					existing, err := os.ReadFile(dest)
					if err != nil {
						continue // not installed here — not an error
					}
					data, err := f.content()
					if err != nil {
						continue
					}
					if bytes.Equal(existing, data) {
						fmt.Printf("ok:    %s\n", dest)
					} else {
						fmt.Printf("stale: %s (reinstall with `tcrit install --force %s`)\n", dest, name)
						stale++
					}
				}
			}
		}
		if stale > 0 {
			return fmt.Errorf("%d stale integration file(s)", stale)
		}
		fmt.Println("All installed integrations are up to date.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(checkCmd)
	installCmd.Flags().BoolVarP(&installForce, "force", "f", false, "overwrite files that differ")
}
