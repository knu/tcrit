package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed skill/tcrit-review/SKILL.md skill/tcrit-plan-review/SKILL.md skill/tcrit-code-review/SKILL.md
var skillContent embed.FS

var setupProject bool
var setupForce bool

// skills to install: directory name -> display name
var skillsToInstall = []struct {
	dir  string
	name string
}{
	{"tcrit-review", "tcrit-review"},
	{"tcrit-plan-review", "tcrit-plan-review"},
	{"tcrit-code-review", "tcrit-code-review"},
}

var setupClaudeCmd = &cobra.Command{
	Use:   "setup-claude",
	Short: "Install Claude Code skills for TCrit review workflow",
	Long:  "Installs /tcrit-review, /tcrit-plan-review, and /tcrit-code-review skills to ~/.claude/skills/ (or .claude/skills/ with --project). Alternative to installing the tcrit plugin via /plugin install.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var baseDir string

		if setupProject {
			baseDir = filepath.Join(".claude", "skills")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("could not determine home directory: %w", err)
			}
			baseDir = filepath.Join(home, ".claude", "skills")
		}

		scope := "globally"
		if setupProject {
			scope = "for this project"
		}

		for _, skill := range skillsToInstall {
			targetDir := filepath.Join(baseDir, skill.dir)
			targetPath := filepath.Join(targetDir, "SKILL.md")

			if !setupForce {
				if _, err := os.Stat(targetPath); err == nil {
					fmt.Printf("Skipping %s (already exists, use --force to overwrite)\n", skill.name)
					continue
				}
			}

			content, err := skillContent.ReadFile(filepath.Join("skill", skill.dir, "SKILL.md"))
			if err != nil {
				return fmt.Errorf("reading embedded skill %s: %w", skill.name, err)
			}

			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", targetDir, err)
			}

			if err := os.WriteFile(targetPath, content, 0644); err != nil {
				return fmt.Errorf("writing skill file %s: %w", skill.name, err)
			}

			fmt.Printf("Installed /%s %s to %s\n", skill.name, scope, targetPath)
		}

		fmt.Println("\nAvailable skills:")
		fmt.Println("  /tcrit-review        — Routes to code or plan review")
		fmt.Println("  /tcrit-code-review   — Multi-file code review")
		fmt.Println("  /tcrit-plan-review   — Single-file document review")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupClaudeCmd)
	setupClaudeCmd.Flags().BoolVar(&setupProject, "project", false, "install to .claude/skills/ in the current directory instead of globally")
	setupClaudeCmd.Flags().BoolVar(&setupForce, "force", false, "overwrite existing skill file")
}
