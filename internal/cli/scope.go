package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
