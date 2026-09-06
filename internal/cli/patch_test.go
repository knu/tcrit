package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/knu/tcrit/internal/config"
	"github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

const addedPatch = "diff --git a/a.txt b/a.txt\nnew file mode 100644\n--- /dev/null\n+++ b/a.txt\n@@ -0,0 +1 @@\n+snapshot\n"

func TestDiffReviewSessionAndDetachedCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	p, err := git.ParsePatch(strings.NewReader(addedPatch))
	if err != nil {
		t.Fatal(err)
	}
	mode := &reviewMode{patch: p, files: p.Changes()}
	sess, err := openReviewSession(&config.Config{}, mode)
	if err != nil {
		t.Fatal(err)
	}
	code, err := review.OpenCodeSession("")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Key == code.Key || mode.diffSession != sess.Key || mode.internalMode() != "diff" || len(sess.CJ.CliArgs) != 1 || sess.CJ.CliArgs[0] != "--diff" {
		t.Fatalf("incorrect diff session: %+v", sess)
	}
	entry, err := review.ReadSessionEntry(sess.Key)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := review.OpenSessionFromEntry(*entry)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := git.LoadPatch(reopened.DiffPath())
	if err != nil || loaded.Files[0].Content != "snapshot\n" {
		t.Fatalf("snapshot = %+v, error = %v", loaded, err)
	}
	original := resolveExec
	resolveExec = func() (string, error) { return "/a path/tcrit", nil }
	t.Cleanup(func() { resolveExec = original })
	cmd, err := buildTUICommand(mode)
	if err != nil || !strings.Contains(cmd, "'/a path/tcrit' _tui --diff-session '"+sess.Key+"'") {
		t.Fatalf("detached command = %q, %v", cmd, err)
	}
	if got := nextRoundCommand(sess, mode); !strings.Contains(got, "--diff") {
		t.Fatalf("next round does not request updated diff: %q", got)
	}
}

func TestDiffInputArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	path := "changes 日本語.diff"
	if err := os.WriteFile(path, []byte(addedPatch), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, diff := os.Stdin, reviewDiff
	t.Cleanup(func() { os.Stdin, reviewDiff = stdin, diff })
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		args []string
		fail bool
	}{
		{[]string{"--diff"}, false},
		{[]string{"--diff=-"}, false},
		{[]string{"--diff", "-"}, false},
		{[]string{"--diff=" + path}, false},
		{[]string{"--diff", "./" + path}, false},
		{[]string{"--diff=" + abs}, false},
		{[]string{"--diff="}, true},
		{[]string{"--diff=missing.diff"}, true},
		{[]string{"--diff=" + path, path}, true},
		{[]string{"--diff", path, path}, true},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := f.Close(); err != nil {
					t.Error(err)
				}
			})
			os.Stdin = f
			cmd := &cobra.Command{
				Args: cobra.MaximumNArgs(1), SilenceUsage: true, SilenceErrors: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					mode, err := resolveReviewMode(args, &config.Config{})
					if err == nil && (mode.patch == nil || len(mode.files) != 1) {
						t.Fatalf("incorrect diff mode: %+v", mode)
					}
					return err
				},
			}
			addDiffFlag(cmd)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); (err != nil) != tt.fail {
				t.Fatalf("error = %v, want failure %v", err, tt.fail)
			}
		})
	}
}

func TestResolveDiffFromStdin(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join(t.TempDir(), "input.diff")
	if err := os.WriteFile(path, []byte(addedPatch), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	stdin, diff, code, staged, base := os.Stdin, reviewDiff, reviewCode, reviewStaged, reviewBase
	os.Stdin, reviewDiff, reviewCode, reviewStaged, reviewBase = f, "-", false, false, ""
	t.Cleanup(func() { os.Stdin, reviewDiff, reviewCode, reviewStaged, reviewBase = stdin, diff, code, staged, base })
	mode, err := resolveReviewMode(nil, &config.Config{BaseBranch: "ignore-me"})
	if err != nil || mode.patch == nil || len(mode.files) != 1 {
		t.Fatalf("mode = %+v, %v", mode, err)
	}
	if _, err := resolveReviewMode([]string{"file"}, &config.Config{}); err == nil {
		t.Fatal("accepted document with --diff")
	}
	reviewStaged = true
	if _, err := resolveReviewMode(nil, &config.Config{}); err == nil {
		t.Fatal("accepted staged with --diff")
	}
	reviewStaged, reviewBase = false, "main"
	if _, err := resolveReviewMode(nil, &config.Config{}); err == nil {
		t.Fatal("accepted base with --diff")
	}
}
