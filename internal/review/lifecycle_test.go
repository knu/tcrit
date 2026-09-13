package review

import (
	"testing"
)

func TestNewSessionsKeepIdenticalTasksIndependent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	meta := SessionEntry{CWD: t.TempDir(), Mode: "git", Args: []string{"--scope", "staged"}}
	a, err := NewSession("", meta)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSession("", meta)
	if err != nil {
		t.Fatal(err)
	}
	if a.Key == b.Key || a.Dir == b.Dir {
		t.Fatal("independent tasks share storage")
	}
	a.SetFileComments("a.go", "modified", []Comment{{ID: "c_one", Body: "retain me"}})
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if len(b.FileComments("a.go")) != 0 {
		t.Fatal("comment leaked into another task")
	}
	entry, err := ReadSessionEntry(a.Key)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSessionFromEntry(*entry)
	if err != nil || len(loaded.FileComments("a.go")) != 1 {
		t.Fatalf("saved task lost: %v", err)
	}
}

func TestRestartPreservesFeedbackAndAdvancesOnlyFinishedRounds(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, err := NewSession("", SessionEntry{CWD: t.TempDir(), Mode: "files"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	files := func(content string) map[string]SnapshotFile {
		return map[string]SnapshotFile{"a.md": {Content: content}}
	}
	if err := s.BeginRound(files("first\ntarget\n"), ""); err != nil {
		t.Fatal(err)
	}
	s.SetFileComments("a.md", "", []Comment{{ID: "c_original", Body: "fix", StartLine: 2, EndLine: 2, Anchor: "target", ReviewRound: 1}})
	s.CJ.RoundState.NewFeedback = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reopen := func() *Session {
		t.Helper()
		e, err := ReadSessionEntry(s.Key)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := OpenSessionFromEntry(*e)
		if err != nil {
			t.Fatal(err)
		}
		return loaded
	}
	s = reopen()
	if err := s.BeginRound(files("first\ntarget\n"), ""); err != nil {
		t.Fatal(err)
	}
	if s.CJ.ReviewRound != 1 || !s.CJ.RoundState.NewFeedback || s.FileComments("a.md")[0].ID != "c_original" {
		t.Fatal("interrupted round was treated as completed")
	}
	c := s.CJ.Files["a.md"]
	c.Comments[0].Replies = []Reply{{ID: "rp_one", Body: "done", Author: "Agent"}}
	s.CJ.Files["a.md"] = c
	s.CJ.RoundState.Finished = true
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s = reopen()
	if err := s.BeginRound(files("new\nfirst\ntarget\n"), ""); err != nil {
		t.Fatal(err)
	}
	comment := s.FileComments("a.md")[0]
	if s.CJ.ReviewRound != 2 || s.CJ.RoundState.NewFeedback || s.CJ.RoundState.Finished || comment.StartLine != 3 || len(comment.Replies) != 1 {
		t.Fatalf("round was not restored: %+v, %+v", s.CJ.RoundState, comment)
	}
	s = reopen()
	if err := s.BeginRound(files("new\nfirst\ntarget\n"), ""); err != nil {
		t.Fatal(err)
	}
	if s.CJ.ReviewRound != 2 {
		t.Fatal("restart advanced the round twice")
	}
}
