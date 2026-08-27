package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/review"
)

var clearCode bool
var clearAll bool

var clearCmd = &cobra.Command{
	Use:   "clear [file]",
	Short: "Clear all comments for a document or code review session",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if clearAll {
			if clearCode || len(args) > 0 {
				return fmt.Errorf("--all cannot be combined with --code or a file argument")
			}
			return runClearAll()
		}

		if clearCode {
			return runCodeClear()
		}

		if len(args) == 0 {
			return fmt.Errorf("file argument required (use --code for code review session, --all to reset all review state)")
		}

		filePath := args[0]

		state, err := review.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		count := len(state.Comments)
		state.Comments = []review.Comment{}

		if err := review.Save(state); err != nil {
			return fmt.Errorf("saving review: %w", err)
		}

		fmt.Printf("Cleared %d comment(s) for %s\n", count, filePath)
		return nil
	},
}

func runCodeClear() error {
	session, err := review.LoadSession()
	if err != nil {
		return err
	}

	total := 0
	for _, file := range session.Files {
		state, err := review.Load(file)
		if err != nil {
			continue
		}
		count := len(state.Comments)
		if count == 0 {
			continue
		}
		state.Comments = []review.Comment{}
		if err := review.Save(state); err != nil {
			return fmt.Errorf("saving review for %s: %w", file, err)
		}
		total += count
	}

	fmt.Printf("Cleared %d comment(s) across %d file(s)\n", total, len(session.Files))
	return nil
}

// runClearAll deletes all saved review state (per-file reviews and the code
// review session manifest), keeping .crit/.gitignore and anything else in
// .crit intact.  Use it when starting a fresh review task.
func runClearAll() error {
	removed := 0

	reviewsDir := filepath.Join(".crit", "reviews")
	entries, err := os.ReadDir(reviewsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", reviewsDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() ||
			(!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".json")) {
			continue
		}
		if err := os.Remove(filepath.Join(reviewsDir, name)); err != nil {
			return fmt.Errorf("removing review state: %w", err)
		}
		removed++
	}

	sessionPath := filepath.Join(".crit", "code-review.yaml")
	if err := os.Remove(sessionPath); err == nil {
		removed++
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("removing session: %w", err)
	}

	fmt.Printf("Removed %d review state file(s)\n", removed)
	return nil
}

func init() {
	rootCmd.AddCommand(clearCmd)
	clearCmd.Flags().BoolVar(&clearCode, "code", false, "clear comments for all files in the code review session")
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "delete all saved review state, keeping .crit/.gitignore")
}
