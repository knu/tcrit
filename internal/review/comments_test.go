package review

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func newTestSession(t *testing.T) *Session {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	sess, err := OpenSession("", "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func TestAppendReplyResolveSemantics(t *testing.T) {
	sess := newTestSession(t)
	sess.CJ.ReviewRound = 3
	sess.SetFileComments("a.go", "", []Comment{{ID: "c_1", StartLine: 1, EndLine: 1, Body: "fix"}})

	if err := sess.AppendReply("c_1", "done", "AI", "", true, ""); err != nil {
		t.Fatalf("resolving reply: %v", err)
	}
	c := sess.FileComments("a.go")[0]
	if !c.Resolved || c.ResolvedRound != 3 {
		t.Errorf("expected resolved at round 3, got %+v", c)
	}
	if len(c.Replies) != 1 || c.Replies[0].Body != "done" || c.Replies[0].ReviewRound != 3 {
		t.Errorf("reply not recorded: %+v", c.Replies)
	}

	// A non-resolving reply unresolves a previously-resolved comment.
	if err := sess.AppendReply("c_1", "wait, one more thing", "AI", "", false, ""); err != nil {
		t.Fatalf("non-resolving reply: %v", err)
	}
	c = sess.FileComments("a.go")[0]
	if c.Resolved || c.ResolvedRound != 0 {
		t.Errorf("expected unresolved after non-resolving reply, got %+v", c)
	}
}

func TestAppendReplyReviewLevelAndErrors(t *testing.T) {
	sess := newTestSession(t)
	sess.AppendReviewComment("overall", "Human", "")
	id := sess.CJ.ReviewComments[0].ID

	if err := sess.AppendReply(id, "ack", "AI", "", false, ""); err != nil {
		t.Fatalf("review-level reply: %v", err)
	}
	if len(sess.CJ.ReviewComments[0].Replies) != 1 {
		t.Error("review-level reply not recorded")
	}

	err := sess.AppendReply("c_nope", "x", "AI", "", false, "")
	var notFound *CommentNotFoundError
	if err == nil || !strings.Contains(err.Error(), "c_nope") {
		t.Errorf("expected not-found error, got %v", err)
	} else if !errors.As(err, &notFound) {
		t.Errorf("expected CommentNotFoundError, got %T", err)
	}

	// Duplicate IDs across files require --path.
	sess.SetFileComments("a.go", "", []Comment{{ID: "c_dup", StartLine: 1, EndLine: 1}})
	sess.SetFileComments("b.go", "", []Comment{{ID: "c_dup", StartLine: 2, EndLine: 2}})
	if err := sess.AppendReply("c_dup", "x", "AI", "", false, ""); err == nil ||
		!strings.Contains(err.Error(), "--path") {
		t.Errorf("expected disambiguation error, got %v", err)
	}
	if err := sess.AppendReply("c_dup", "x", "AI", "", false, "b.go"); err != nil {
		t.Errorf("path-filtered reply failed: %v", err)
	}
}

func TestParseLineSpec(t *testing.T) {
	tests := []struct {
		spec       string
		start, end int
		wantErr    bool
	}{
		{"42", 42, 42, false},
		{"45-47", 45, 47, false},
		{"x", 0, 0, true},
		{"1-x", 0, 0, true},
	}
	for _, tt := range tests {
		start, end, err := ParseLineSpec(tt.spec)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseLineSpec(%q) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && (start != tt.start || end != tt.end) {
			t.Errorf("ParseLineSpec(%q) = %d,%d, want %d,%d", tt.spec, start, end, tt.start, tt.end)
		}
	}
}

func TestIsAbsoluteOrTraversal(t *testing.T) {
	for _, p := range []string{"/etc/passwd", "../up", "..", "sub/../../out"} {
		if !IsAbsoluteOrTraversal(p) {
			t.Errorf("expected %q to be rejected", p)
		}
	}
	for _, p := range []string{"a.go", "sub/a.go", "./a.go", "sub/../a.go"} {
		if IsAbsoluteOrTraversal(p) {
			t.Errorf("expected %q to be accepted", p)
		}
	}
}

func TestBulkEntryLineFieldForms(t *testing.T) {
	var entries []BulkCommentEntry
	input := `[
		{"file": "a.go", "line": 42, "body": "int line"},
		{"file": "a.go", "line": "45-47", "body": "range line"},
		{"body": "review level"},
		{"path": "b.go", "body": "file level via path alias"},
		{"reply_to": "c_1", "body": "a reply", "resolve": true}
	]`
	if err := json.Unmarshal([]byte(input), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entries[0].Line != 42 || entries[1].LineSpec != "45-47" {
		t.Errorf("line forms not parsed: %+v", entries[:2])
	}

	sess := newTestSession(t)
	sess.SetFileComments("a.go", "", []Comment{{ID: "c_1", StartLine: 1, EndLine: 1}})
	if err := sess.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	stats, err := sess.ApplyBulk(entries, "AI", "")
	if err != nil {
		t.Fatalf("ApplyBulk: %v", err)
	}
	if stats.Comments != 4 || stats.Replies != 1 {
		t.Errorf("stats = %+v, want 4 comments / 1 reply", stats)
	}

	if got := len(sess.CJ.ReviewComments); got != 1 {
		t.Errorf("review comments = %d, want 1", got)
	}
	aComments := sess.FileComments("a.go")
	if len(aComments) != 3 {
		t.Fatalf("a.go comments = %d, want 3", len(aComments))
	}
	if aComments[1].StartLine != 42 || aComments[2].StartLine != 45 || aComments[2].EndLine != 47 {
		t.Errorf("line comments misparsed: %+v", aComments[1:])
	}
	if !aComments[0].Resolved {
		t.Error("reply with resolve=true should resolve the parent")
	}
	bComments := sess.FileComments("b.go")
	if len(bComments) != 1 || bComments[0].Scope != "file" {
		t.Errorf("path alias should create a file-level comment, got %+v", bComments)
	}
}

func TestBulkEntryValidation(t *testing.T) {
	tests := []struct {
		name  string
		entry BulkCommentEntry
		want  string
	}{
		{"missing body", BulkCommentEntry{File: "a.go", Line: 1}, "body is required"},
		{"line without file", BulkCommentEntry{Line: 5, Body: "x"}, "file is required"},
		{"file without line", BulkCommentEntry{File: "a.go", Body: "x"}, "line must be > 0"},
		{"traversal", BulkCommentEntry{File: "../a.go", Line: 1, Body: "x"}, "must be relative"},
		{"windows traversal", BulkCommentEntry{File: `sub\..\..\etc`, Line: 1, Body: "x"}, "must be relative"},
		{"scope review with file", BulkCommentEntry{Scope: "x", File: "a.go", Body: "b"}, "line must be > 0"},
	}
	for _, tt := range tests {
		sess := newTestSession(t)
		err := sess.ApplyBulkEntry(0, tt.entry, "AI", "")
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: error = %v, want containing %q", tt.name, err, tt.want)
		}
	}
}
