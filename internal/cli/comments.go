package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/review"
)

var (
	commentsSession string
	commentsOutput  string
	commentsJSON    bool
	commentsAll     bool
)

var commentsCmd = &cobra.Command{
	Use:   "comments [<review-path>]",
	Short: "List review comments, review-level first",
	Long: `List review comments: review-level comments first, then files in
path order with file-level comments before line-level ones.  Only
unresolved comments are shown unless --all is given.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadCurrent()
		if err != nil {
			return err
		}
		output := commentsOutput
		if output == "" {
			output = cfg.Output
		}

		var sess *review.Session
		if len(args) == 1 {
			if commentsSession != "" {
				return fmt.Errorf("--session cannot be combined with an explicit review path")
			}
			sess, err = openSessionAtPathArg(args[0])
		} else {
			sess, err = review.ResolveTarget(output, commentsSession)
		}
		if err != nil {
			return err
		}

		entries := sess.CJ.ListComments(!commentsAll)
		if commentsJSON {
			data, err := review.EncodeCommentsJSON(entries)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Println(review.FormatCommentsText(entries, !commentsAll))
		return nil
	},
}

// openSessionAtPathArg accepts a review folder, a review.json path, or a
// data root containing one review, and opens the review found there.
func openSessionAtPathArg(arg string) (*review.Session, error) {
	dir := arg
	if strings.HasSuffix(arg, "review.json") {
		dir = filepath.Dir(arg)
	}
	if _, err := os.Stat(review.JSONPath(dir)); err != nil {
		return nil, fmt.Errorf("no review.json found at %s", arg)
	}
	key := filepath.Base(dir)
	if !review.ValidSessionKey(key) {
		key = ""
	}
	return review.OpenSessionAt(key, dir)
}

func init() {
	rootCmd.AddCommand(commentsCmd)
	commentsCmd.Flags().StringVar(&commentsSession, "session", "", "target a review session by ID")
	commentsCmd.Flags().StringVarP(&commentsOutput, "output", "o", "", "review data root")
	commentsCmd.Flags().BoolVar(&commentsJSON, "json", false, "output as JSON")
	commentsCmd.Flags().BoolVar(&commentsAll, "all", false, "include resolved comments")
}
