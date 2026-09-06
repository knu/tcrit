package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/git"
)

func validateScopeFlags(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("diff") && reviewDiff == "" {
		return fmt.Errorf("--diff requires a file path or -")
	}
	if cmd.Flags().Changed("scope") && reviewScope == "" {
		return fmt.Errorf("--scope requires a value")
	}
	return nil
}

func resolveCodeScope() (*reviewMode, error) {
	scope := reviewScope
	if reviewStaged {
		if scope != "" && scope != "staged" {
			return nil, fmt.Errorf("--staged conflicts with --scope=%s", scope)
		}
		scope = "staged"
	}
	if scope == "" {
		scope = "all"
	}
	source := git.ReviewSource{Scope: scope, Base: "HEAD"}
	var err error
	if scope != "all" && scope != "staged" && scope != "unstaged" {
		source, err = git.ResolveRange(scope)
		if err != nil {
			return nil, err
		}
	}
	files, err := source.Files()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no changes in the selected scope")
	}
	return &reviewMode{files: files, ref: source.Base, staged: source.Scope == "staged", source: &source}, nil
}
