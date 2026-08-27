package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/review"
)

var statusCode bool

// docStatus is the JSON shape for a single-document review status.
type docStatus struct {
	File     string           `json:"file"`
	Comments []review.Comment `json:"comments"`
}

var statusCmd = &cobra.Command{
	Use:   "status [file]",
	Short: "Show review status as JSON",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}

		if statusCode {
			return runCodeStatus(cfg)
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for aggregate code review status)")
		}

		filePath := args[0]

		sess, err := review.OpenDocSession(cfg.Output, filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		comments := sess.FileComments(filePath)
		if comments == nil {
			comments = []review.Comment{}
		}
		return printJSON(docStatus{File: filePath, Comments: comments})
	},
}

func runCodeStatus(cfg *config.Config) error {
	status, err := review.AggregateStatus(cfg.Output)
	if err != nil {
		return err
	}
	return printJSON(status)
}

func printJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusCode, "code", false, "show aggregate status for the code review session")
}
