package review

import (
	"strings"

	"github.com/knu/tcrit/internal/diff"
)

// This file ports crit's round carry-forward: when a new round starts,
// every comment is re-minted with a new ID and CarriedForward set, its line
// range is remapped through an LCS diff of the file contents, and the
// stored anchor text verifies (or corrects) the result, flagging Drifted
// when the anchored text no longer exists.

// CarryForwardFile maps a file's comments onto the file's new content.
// prevContent is the content the comments were authored against; now stamps
// UpdatedAt on the re-minted comments.
func CarryForwardFile(comments []Comment, prevContent, newContent, now string) []Comment {
	if len(comments) == 0 {
		return comments
	}

	entries := diff.ComputeLineDiff(prevContent, newContent)
	lineMap := diff.MapOldLineToNew(entries)
	newLines := diff.SplitLines(newContent)
	maxLine := len(newLines)
	if maxLine == 0 {
		maxLine = 1
	}

	out := make([]Comment, 0, len(comments))
	for _, c := range comments {
		carried := carryForwardComment(c, now)

		// File-scoped comments have no line references; old-side comments
		// reference the diff base, which does not move between rounds.
		if c.Scope == "file" || c.Side == "old" {
			out = append(out, carried)
			continue
		}

		start, end := remapLines(lineMap, c.StartLine, c.EndAt(), maxLine)
		if c.Anchor != "" {
			var drifted int
			start, end, drifted = verifyAndCorrectPosition(newLines, c.Anchor, start, end)
			carried.Drifted = drifted != 0
		}
		carried.StartLine = start
		carried.EndLine = end
		out = append(out, carried)
	}
	return out
}

// carryForwardComment re-mints a comment for the next round: a fresh ID,
// CarriedForward forced true, UpdatedAt restamped, and everything else —
// including the original authoring round, resolution state, and replies —
// preserved.
func carryForwardComment(old Comment, now string) Comment {
	return Comment{
		ID:             RandomCommentID(),
		StartLine:      old.StartLine,
		EndLine:        old.EndLine,
		Side:           old.Side,
		Body:           old.Body,
		Quote:          old.Quote,
		QuoteOffset:    old.QuoteOffset,
		Anchor:         old.Anchor,
		Drifted:        old.Drifted,
		DriftedOnRound: old.DriftedOnRound,
		Author:         old.Author,
		UserID:         old.UserID,
		Scope:          old.Scope,
		CreatedAt:      old.CreatedAt,
		UpdatedAt:      now,
		Resolved:       old.Resolved,
		ResolvedRound:  old.ResolvedRound,
		CarriedForward: true,
		ReviewRound:    old.ReviewRound,
		Replies:        old.Replies,

		GitHubID:           old.GitHubID,
		GitLabNoteID:       old.GitLabNoteID,
		GitLabDiscussionID: old.GitLabDiscussionID,
		GitLabResolved:     old.GitLabResolved,
		LastPushedBodyHash: old.LastPushedBodyHash,
		HeadSHA:            old.HeadSHA,
		DiffScope:          old.DiffScope,
		FocusKey:           old.FocusKey,
	}
}

// remapLines translates old start/end line numbers through the LCS line map,
// falling back to the original positions and clamping to [1, maxLine].
func remapLines(lineMap map[int]int, oldStart, oldEnd, maxLine int) (int, int) {
	s := lineMap[oldStart]
	e := lineMap[oldEnd]
	if s == 0 {
		s = oldStart
	}
	if e == 0 {
		e = oldEnd
	}
	if s > maxLine {
		s = maxLine
	}
	if e > maxLine {
		e = maxLine
	}
	if s < 1 {
		s = 1
	}
	if e < s {
		e = s
	}
	return s, e
}

// verifyAndCorrectPosition checks whether the LCS-remapped position still
// points at the anchor text. If not, it searches the new content for the
// anchor. Returns the corrected (start, end) and whether the comment drifted.
func verifyAndCorrectPosition(newLines []string, anchor string, lcsStart, lcsEnd int) (start, end, drifted int) {
	anchorLines := strings.Split(anchor, "\n")
	anchorLen := len(anchorLines)

	// Check if the LCS position still matches.
	if lcsStart >= 1 && lcsStart+anchorLen-1 <= len(newLines) {
		candidate := strings.Join(newLines[lcsStart-1:lcsStart+anchorLen-1], "\n")
		if candidate == anchor {
			return lcsStart, lcsStart + anchorLen - 1, 0
		}
		// Edited-but-recognizable: if LCS predicts the same row and the line
		// is still close enough to the original, treat as anchored. Avoids
		// false drift when text was appended/trimmed/tweaked in place.
		if anchorSimilar(candidate, anchor) {
			return lcsStart, lcsStart + anchorLen - 1, 0
		}
	}

	// LCS position doesn't match — search the entire file.
	if found := findAnchorInLines(newLines, anchor, lcsStart); found > 0 {
		return found, found + anchorLen - 1, 0
	}

	// Anchor not found anywhere — mark drifted, keep the LCS position.
	return lcsStart, lcsEnd, 1
}

// findAnchorInLines searches for the anchor text in lines.  It returns the
// 1-indexed start line of the exact match closest to preferredStart, or 0.
func findAnchorInLines(lines []string, anchor string, preferredStart int) int {
	anchorLines := strings.Split(anchor, "\n")
	anchorLen := len(anchorLines)
	if anchorLen == 0 || len(lines) < anchorLen {
		return 0
	}

	var matches []int
	for i := 0; i <= len(lines)-anchorLen; i++ {
		if strings.Join(lines[i:i+anchorLen], "\n") == anchor {
			matches = append(matches, i+1)
		}
	}
	if len(matches) == 0 {
		return 0
	}

	best := matches[0]
	bestDist := abs(best - preferredStart)
	for _, m := range matches[1:] {
		if d := abs(m - preferredStart); d < bestDist {
			best = m
			bestDist = d
		}
	}
	return best
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// anchorSimilar reports whether candidate and anchor are close enough to
// treat the comment as still anchored. Catches in-place edits (appended,
// trimmed, or lightly reworded text) that exact match would flag as drifted.
func anchorSimilar(candidate, anchor string) bool {
	a := strings.TrimSpace(candidate)
	b := strings.TrimSpace(anchor)
	if a == b {
		return true
	}
	if a == "" || b == "" {
		return false
	}
	// Common case: text was appended to or trimmed from the anchor line.
	// Gate on a minimum length so trivial anchors (`}`, `return nil`) don't
	// match any longer line that happens to contain them.
	minLen := min(len(a), len(b))
	if minLen >= 8 && (strings.Contains(a, b) || strings.Contains(b, a)) {
		return true
	}
	return levenshteinRatio(a, b) >= 0.7
}

// levenshteinRatio returns 1 - (distance / maxLen), clamped to [0, 1].
func levenshteinRatio(a, b string) float64 {
	ar, br := []rune(a), []rune(b)
	la, lb := len(ar), len(br)
	if la == 0 && lb == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ar, br))/float64(max(la, lb))
}

// levenshtein computes edit distance between two rune slices using a
// rolling two-row buffer. O(la*lb) time, O(min(la,lb)) space.
func levenshtein(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
