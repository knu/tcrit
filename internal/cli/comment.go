package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/document"
	"github.com/knu/tcrit/internal/review"
)

var commentLine int
var commentEndLine int
var commentBody string

var commentCmd = &cobra.Command{
	Use:   "comment <file>",
	Short: "Add an inline comment to a document",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		doc, err := document.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading document: %w", err)
		}

		if commentLine < 1 || commentLine > doc.LineCount() {
			return fmt.Errorf("line %d is out of range (document has %d lines)", commentLine, doc.LineCount())
		}

		if commentBody == "" {
			return fmt.Errorf("--body is required")
		}

		state, err := review.Load(filePath)
		if err != nil {
			return fmt.Errorf("loading review state: %w", err)
		}

		endLine := commentEndLine
		if endLine > 0 && endLine < commentLine {
			return fmt.Errorf("--end-line (%d) must be >= --line (%d)", endLine, commentLine)
		}
		if endLine > doc.LineCount() {
			return fmt.Errorf("end-line %d is out of range (document has %d lines)", endLine, doc.LineCount())
		}

		comment := review.Comment{
			ID:             uuid.NewString()[:8],
			Line:           commentLine,
			EndLine:        endLine,
			ContentSnippet: doc.LineAt(commentLine),
			Body:           commentBody,
			CreatedAt:      time.Now(),
		}

		state.AddComment(comment)

		if err := review.Save(state); err != nil {
			return fmt.Errorf("saving review: %w", err)
		}

		if endLine > 0 {
			fmt.Printf("Comment added at lines %d-%d\n", commentLine, endLine)
		} else {
			fmt.Printf("Comment added at line %d\n", commentLine)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commentCmd)
	commentCmd.Flags().IntVar(&commentLine, "line", 0, "line number to comment on (required)")
	commentCmd.Flags().IntVar(&commentEndLine, "end-line", 0, "end line for multi-line comment (optional)")
	commentCmd.Flags().StringVar(&commentBody, "body", "", "comment text (required)")
	commentCmd.MarkFlagRequired("line")
	commentCmd.MarkFlagRequired("body")
}
