package git

import (
	"reflect"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestIgnoreWhitespace(t *testing.T) {
	for _, tt := range []struct {
		name, hunk, content string
		added               []int
		deleted             map[int][]int
	}{
		{"indent and trailing space", "@@ -1,2 +1,2 @@\n-a b\n-c\n+\ta  b \n+c\t\n", "\ta  b \nc\t\n", nil, nil},
		{"LF to CRLF", "@@ -1,2 +1,2 @@\n-a\n-b\n+a\r\n+b\r\n", "a\r\nb\r\n", nil, nil},
		{"CRLF to LF", "@@ -1,2 +1,2 @@\n-a\r\n-b\r\n+a\n+b\n", "a\nb\n", nil, nil},
		{"CR inside a line", "@@ -1 +1 @@\n-ab\n+a\rb\n", "a\rb\n", nil, nil},
		{"unequal block", "@@ -10,2 +10,3 @@\n-old\n-keep\n+new\n+  keep\n+extra\n", strings.Repeat("\n", 9) + "new\n  keep\nextra\n", []int{10, 12}, map[int][]int{9: {10}}},
		{"reanchor deletion", "@@ -1,3 +1,2 @@\n-a\n-drop\n-b\n+ a\n+ b\n", " a\n b\n", nil, map[int][]int{1: {2}}},
		{"added blank line", "@@ -1 +1,2 @@\n a\n+ \n", "a\n \n", []int{2}, nil},
		{"deleted file", "@@ -1,2 +0,0 @@\n-a\n- \n", "", nil, map[int][]int{0: {1, 2}}},
		{"unicode space remains significant", "@@ -1 +1 @@\n-ab\n+a\u00a0b\n", "a\u00a0b\n", []int{1}, map[int][]int{0: {1}}},
		{"space within text", "@@ -1 +1 @@\n-a b\n+ab\n", "ab\n", nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files, _, err := gitdiff.Parse(strings.NewReader("diff --git a/f b/f\n--- a/f\n+++ b/f\n" + tt.hunk))
			if err != nil {
				t.Fatal(err)
			}
			original := diffInfo(files)
			before := diffInfo(files)
			got := original.IgnoreWhitespace(strings.Split(tt.content, "\n"))
			if len(got.ChangedLines) != len(tt.added) {
				t.Fatalf("added = %v, want %v", got.ChangedLines, tt.added)
			}
			for _, line := range tt.added {
				if !got.ChangedLines[line] {
					t.Errorf("missing added line %d", line)
				}
			}
			if len(got.DeletedAfter) != len(tt.deleted) {
				t.Fatalf("deleted = %v, want %v", got.DeletedAfter, tt.deleted)
			}
			for anchor, want := range tt.deleted {
				var nums []int
				for _, line := range got.DeletedAfter[anchor] {
					nums = append(nums, line.OldLineNum)
				}
				if !reflect.DeepEqual(nums, want) {
					t.Errorf("deletions at %d = %v, want %v", anchor, nums, want)
				}
			}
			if !reflect.DeepEqual(original, before) {
				t.Fatal("original diff mutated")
			}
		})
	}
}

func TestWhitespaceInlineDiff(t *testing.T) {
	before, after := "\t値 = old  \r", " 値=new\t"
	old, new := whitespaceInlineDiff(before, after)
	for i, segments := range [][]InlineSegment{old, new} {
		var full, changed strings.Builder
		for _, segment := range segments {
			full.WriteString(segment.Content)
			if segment.Changed {
				changed.WriteString(segment.Content)
			}
		}
		if full.String() != []string{before, after}[i] || changed.String() != []string{"old", "new"}[i] {
			t.Fatalf("segments lose content or highlight whitespace: %+v", segments)
		}
	}
}
