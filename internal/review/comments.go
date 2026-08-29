package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The mutators in this file mirror crit's comment CLI semantics
// (https://github.com/tomasz-tomczyk/crit) so agent-facing behavior matches.

// AppendReviewComment adds a review-level comment.
func (s *Session) AppendReviewComment(body, author, userID string) {
	now := Now()
	s.CJ.ReviewComments = append(s.CJ.ReviewComments, Comment{
		ID:          RandomReviewCommentID(),
		Body:        body,
		Author:      author,
		UserID:      userID,
		Scope:       "review",
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewRound: s.CJ.ReviewRound,
	})
}

// AppendFileComment adds a file-level comment (no line reference).
func (s *Session) AppendFileComment(path, body, author, userID string) {
	now := Now()
	c := Comment{
		ID:          RandomCommentID(),
		Body:        body,
		Author:      author,
		UserID:      userID,
		Scope:       "file",
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewRound: s.CJ.ReviewRound,
	}
	s.SetFileComments(path, "", append(s.FileComments(path), c))
}

// AppendLineComment adds a line-level comment, stamping the drift-correction
// anchor from the file's current content on disk when readable.
func (s *Session) AppendLineComment(path string, startLine, endLine int, body, author, userID string) {
	now := Now()
	c := Comment{
		ID:          RandomCommentID(),
		StartLine:   startLine,
		EndLine:     endLine,
		Anchor:      readAnchorFromDisk(path, startLine, endLine),
		Body:        body,
		Author:      author,
		UserID:      userID,
		Scope:       "line",
		CreatedAt:   now,
		UpdatedAt:   now,
		ReviewRound: s.CJ.ReviewRound,
	}
	s.SetFileComments(path, "", append(s.FileComments(path), c))
}

// readAnchorFromDisk returns the full text of lines start..end of the file,
// or "" when the file cannot be read.
func readAnchorFromDisk(path string, start, end int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if start < 1 || start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}

// AppendReply adds a reply to the comment with the given ID, searching
// review-level comments first and then file comments.  filterPath, when
// non-empty, restricts the file search to one path (for duplicated IDs).
//
// Following crit: a resolving reply stamps Resolved/ResolvedRound; a
// non-resolving reply *un*resolves a previously-resolved comment so the new
// reply is not hidden by the resolution filter.
func (s *Session) AppendReply(commentID, body, author, userID string, resolve bool, filterPath string) error {
	now := Now()
	reply := Reply{
		ID:          RandomReplyID(),
		Body:        body,
		Author:      author,
		UserID:      userID,
		CreatedAt:   now,
		ReviewRound: s.CJ.ReviewRound,
	}

	for i, c := range s.CJ.ReviewComments {
		if c.ID != commentID {
			continue
		}
		s.CJ.ReviewComments[i].Replies = append(s.CJ.ReviewComments[i].Replies, reply)
		s.CJ.ReviewComments[i].UpdatedAt = now
		applyResolve(&s.CJ.ReviewComments[i], resolve, s.CJ.ReviewRound)
		return nil
	}

	var foundPaths []string
	for filePath, cf := range s.CJ.Files {
		if filterPath != "" && filePath != NormalizePath(filterPath) {
			continue
		}
		for i, c := range cf.Comments {
			if c.ID != commentID {
				continue
			}
			foundPaths = append(foundPaths, filePath)
			if len(foundPaths) == 1 {
				cf.Comments[i].Replies = append(cf.Comments[i].Replies, reply)
				cf.Comments[i].UpdatedAt = now
				applyResolve(&cf.Comments[i], resolve, s.CJ.ReviewRound)
				s.CJ.Files[filePath] = cf
			}
		}
	}

	switch {
	case len(foundPaths) == 0:
		return &CommentNotFoundError{ID: commentID}
	case len(foundPaths) > 1:
		return fmt.Errorf("comment %q found in multiple files (%s); use --path <file> to disambiguate",
			commentID, strings.Join(foundPaths, ", "))
	}
	return nil
}

func applyResolve(c *Comment, resolve bool, round int) {
	if resolve {
		c.Resolved = true
		c.ResolvedRound = round
	} else {
		c.Resolved = false
		c.ResolvedRound = 0
	}
}

// CommentNotFoundError reports a reply target missing from a review file.
type CommentNotFoundError struct{ ID string }

func (e *CommentNotFoundError) Error() string {
	return fmt.Sprintf("comment %q not found", e.ID)
}

// ContainsCommentID reports whether the review holds a comment with this ID.
func (cj *CritJSON) ContainsCommentID(id string) bool {
	for _, c := range cj.ReviewComments {
		if c.ID == id {
			return true
		}
	}
	for _, f := range cj.Files {
		for _, c := range f.Comments {
			if c.ID == id {
				return true
			}
		}
	}
	return false
}

// ParseLineSpec parses "42" or "45-47" into a start/end line pair.
func ParseLineSpec(spec string) (start, end int, err error) {
	if dashIdx := strings.Index(spec, "-"); dashIdx >= 0 {
		s, err1 := strconv.Atoi(spec[:dashIdx])
		e, err2 := strconv.Atoi(spec[dashIdx+1:])
		if err1 != nil {
			return 0, 0, err1
		}
		if err2 != nil {
			return 0, 0, err2
		}
		return s, e, nil
	}
	n, err := strconv.Atoi(spec)
	if err != nil {
		return 0, 0, err
	}
	return n, n, nil
}

// IsAbsoluteOrTraversal rejects absolute paths and paths escaping the
// working tree.  The check runs on the raw input before Clean, following
// crit, so Windows-style prefixes cannot slip through.
func IsAbsoluteOrTraversal(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || filepath.IsAbs(p) {
		return true
	}
	cleaned := filepath.Clean(p)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, `..\`)
}

// BulkCommentEntry is one element of the `comment --json` input array.
type BulkCommentEntry struct {
	File     string `json:"file,omitempty"`
	Path     string `json:"path,omitempty"`     // alias for File
	Line     int    `json:"-"`                  // parsed from "line" when int
	LineSpec string `json:"-"`                  // parsed from "line" when string ("45-47")
	EndLine  int    `json:"end_line,omitempty"` // defaults to Line
	Body     string `json:"body"`
	Author   string `json:"author,omitempty"` // per-entry override; falls back to global
	Scope    string `json:"scope,omitempty"`  // "review", "file", or "" (inferred)
	ReplyTo  string `json:"reply_to,omitempty"`
	Resolve  bool   `json:"resolve,omitempty"`
}

// UnmarshalJSON accepts the "line" field as either an int (42) or a string
// line spec ("45-47").
func (e *BulkCommentEntry) UnmarshalJSON(data []byte) error {
	type alias BulkCommentEntry
	aux := &struct {
		Line json.RawMessage `json:"line,omitempty"`
		*alias
	}{alias: (*alias)(e)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if len(aux.Line) == 0 || string(aux.Line) == "null" {
		return nil
	}
	var lineInt int
	if err := json.Unmarshal(aux.Line, &lineInt); err == nil {
		e.Line = lineInt
		return nil
	}
	var lineStr string
	if err := json.Unmarshal(aux.Line, &lineStr); err == nil {
		e.LineSpec = lineStr
		return nil
	}
	return fmt.Errorf("line must be int or string, got %s", aux.Line)
}

// ApplyBulkEntry routes one bulk entry to the matching authoring helper.
// globalAuthor fills entries without their own author; userID always comes
// from local config, never the payload (a payload user_id would be a spoof
// vector).
func (s *Session) ApplyBulkEntry(i int, e BulkCommentEntry, globalAuthor, userID string) error {
	if e.Body == "" {
		return fmt.Errorf("entry %d: body is required", i)
	}
	author := e.Author
	if author == "" {
		author = globalAuthor
	}

	if e.ReplyTo != "" {
		if err := s.AppendReply(e.ReplyTo, e.Body, author, userID, e.Resolve, e.File); err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		return nil
	}

	if e.Scope == "review" || (e.File == "" && e.Path == "" && e.Line <= 0 && e.LineSpec == "") {
		if e.Line > 0 || e.LineSpec != "" {
			return fmt.Errorf("entry %d: file is required for new comments", i)
		}
		if e.Scope != "review" && (e.File != "" || e.Path != "") {
			return fmt.Errorf("entry %d: file is required for new comments", i)
		}
		s.AppendReviewComment(e.Body, author, userID)
		return nil
	}

	filePath := e.File
	if filePath == "" {
		filePath = e.Path
	}
	if filePath == "" {
		return fmt.Errorf("entry %d: file is required for new comments", i)
	}
	// Normalize backslashes before the traversal check so Windows-style
	// traversal cannot pass on Unix, following crit.
	normalized := strings.ReplaceAll(filePath, `\`, "/")
	if IsAbsoluteOrTraversal(normalized) {
		return fmt.Errorf("entry %d: path %q must be relative and within the repository", i, filePath)
	}
	cleaned := filepath.ToSlash(filepath.Clean(normalized))

	if e.Scope == "file" {
		s.AppendFileComment(cleaned, e.Body, author, userID)
		return nil
	}

	if e.Line <= 0 && e.LineSpec == "" {
		// The path-alias-implies-file rule: "path" without a line means a
		// file-level comment; bare "file" without a line is an error.
		if e.Path != "" && e.File == "" {
			s.AppendFileComment(cleaned, e.Body, author, userID)
			return nil
		}
		return fmt.Errorf("entry %d: line must be > 0", i)
	}

	startLine, endLine := e.Line, e.EndLine
	if e.LineSpec != "" && startLine == 0 {
		var err error
		startLine, endLine, err = ParseLineSpec(e.LineSpec)
		if err != nil {
			return fmt.Errorf("entry %d: invalid line spec %q", i, e.LineSpec)
		}
	}
	if startLine <= 0 {
		return fmt.Errorf("entry %d: line must be > 0", i)
	}
	if endLine == 0 {
		endLine = startLine
	}
	s.AppendLineComment(cleaned, startLine, endLine, e.Body, author, userID)
	return nil
}

// BulkStats summarizes what a bulk apply added.
type BulkStats struct {
	Comments int
	Replies  int
}

// ApplyBulk validates and applies all entries to this session in one atomic
// write.  Any entry error aborts the batch before anything is saved.
func (s *Session) ApplyBulk(entries []BulkCommentEntry, globalAuthor, userID string) (BulkStats, error) {
	var stats BulkStats
	err := s.Update(func(s *Session) error {
		var err error
		stats, err = s.applyBulk(entries, globalAuthor, userID)
		return err
	})
	if err != nil {
		return BulkStats{}, err
	}
	return stats, nil
}

func (s *Session) applyBulk(entries []BulkCommentEntry, globalAuthor, userID string) (BulkStats, error) {
	if len(entries) == 0 {
		return BulkStats{}, fmt.Errorf("no comment entries provided")
	}
	var stats BulkStats
	for i, e := range entries {
		if err := s.ApplyBulkEntry(i, e, globalAuthor, userID); err != nil {
			return BulkStats{}, err
		}
		if e.ReplyTo != "" {
			stats.Replies++
		} else {
			stats.Comments++
		}
	}
	return stats, nil
}
