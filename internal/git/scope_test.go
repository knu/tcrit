package git

import (
	"os"
	"reflect"
	"testing"
)

func TestReviewSources(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Chdir(dir)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeFile(t, dir, "file 日本語.txt", []byte("base\n"))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "base")
	runGit(t, dir, "tag", "base")
	writeFile(t, dir, "file 日本語.txt", []byte("committed\n"))
	runGit(t, dir, "commit", "-qam", "head")
	runGit(t, dir, "tag", "tip")
	writeFile(t, dir, "file 日本語.txt", []byte("staged\n"))
	runGit(t, dir, "add", ".")
	writeFile(t, dir, "file 日本語.txt", []byte("working\n"))
	writeFile(t, dir, "untracked.txt", []byte("new\n"))
	for _, tc := range []struct {
		scope, content string
		count          int
	}{
		{"all", "working\n", 2}, {"staged", "staged\n", 1}, {"unstaged", "working\n", 2},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			s := ReviewSource{Scope: tc.scope}
			files, err := s.Files()
			if err != nil || len(files) != tc.count {
				t.Fatalf("files=%v err=%v", files, err)
			}
			content, err := s.Content("file 日本語.txt")
			if err != nil || string(content) != tc.content {
				t.Fatalf("content=%q err=%v", content, err)
			}
			diff, err := s.Diff("file 日本語.txt")
			if err != nil || diff == nil || !diff.ChangedLines[1] {
				t.Fatalf("diff=%v err=%v", diff, err)
			}
			if tc.scope == "unstaged" && diff.DeletedAfter[0][0].Content != "staged" {
				t.Fatal("unstaged base is not the index")
			}
		})
	}
	for _, value := range []string{"base..tip", "base...tip", "base..", "base..."} {
		s, err := ResolveRange(value)
		if err != nil {
			t.Fatal(err)
		}
		content, err := s.Content("file 日本語.txt")
		if err != nil || string(content) != "committed\n" {
			t.Fatalf("range content=%q err=%v", content, err)
		}
		files, err := s.Files()
		if err != nil || len(files) != 1 {
			t.Fatalf("range files=%v err=%v", files, err)
		}
	}
	for _, value := range []string{"HEAD", "--output=bad", "missing..HEAD", "base..tip..HEAD", "base....tip"} {
		if _, err := ResolveRange(value); err == nil {
			t.Errorf("accepted invalid range %q", value)
		}
	}
	// A divergent left endpoint distinguishes two-dot and three-dot comparisons.
	runGit(t, dir, "reset", "--hard", "base")
	writeFile(t, dir, "left.txt", []byte("left\n"))
	runGit(t, dir, "add", "left.txt")
	runGit(t, dir, "commit", "-qm", "left")
	runGit(t, dir, "tag", "left")
	two, err := ResolveRange("left..tip")
	if err != nil {
		t.Fatal(err)
	}
	three, err := ResolveRange("left...tip")
	if err != nil {
		t.Fatal(err)
	}
	a, err := two.Files()
	if err != nil {
		t.Fatal(err)
	}
	b, err := three.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || len(b) != 1 || reflect.DeepEqual(a, b) {
		t.Fatalf("two-dot=%v three-dot=%v", a, b)
	}
	for _, source := range []ReviewSource{two, three} {
		out, err := gitCommand("diff", "--name-status", "-z", source.Range, "--")
		if err != nil {
			t.Fatal(err)
		}
		files, err := source.Files()
		if err != nil {
			t.Fatal(err)
		}
		if want := parseNameStatusZ(out); !reflect.DeepEqual(files, want) {
			t.Fatalf("%s: files=%v, git diff=%v", source.Range, files, want)
		}
	}
}
