package tui

import (
	"testing"

	gitpkg "github.com/knu/tcrit/internal/git"
	"github.com/knu/tcrit/internal/review"
)

// restartRound constructs a fresh model from disk, as the next TUI process does.
func restartRound(t *testing.T, old AppModel) AppModel {
	t.Helper()
	cfg := AppConfig{Author: old.author, Staged: old.staged, Source: old.source}
	if old.session != nil {
		if err := old.session.Update(func(s *review.Session) error { s.CJ.RoundState.Finished = true; return nil }); err != nil {
			t.Fatal(err)
		}
		loaded, err := review.OpenSessionAt(old.session.Key, old.session.Dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Session = loaded
	}
	var next AppModel
	if !old.multiFile {
		next = NewApp(old.filePath, cfg)
	} else {
		var files []gitpkg.FileChange
		var err error
		ref := old.baseRef
		switch {
		case old.patch != nil:
			cfg.Patch, err = gitpkg.LoadPatch(old.session.DiffPath())
			if err == nil {
				files = cfg.Patch.Changes()
			}
		case old.source != nil:
			if old.source.Scope == "range" {
				source, resolveErr := gitpkg.ResolveRange(old.source.Range)
				if resolveErr != nil {
					t.Fatal(resolveErr)
				}
				cfg.Source = &source
				ref = source.Base
			}
			files, err = cfg.Source.Files()
		case old.staged:
			files, err = gitpkg.ChangedFilesStaged()
		default:
			files, err = gitpkg.ChangedFilesFrom(ref)
		}
		if err != nil {
			t.Fatal(err)
		}
		next = NewCodeReviewApp(files, ref, cfg)
	}
	next.width, next.height = old.width, old.height
	updated, _ := next.Update(docRenderedMsg{})
	next = updated.(AppModel)
	if next.err != nil {
		t.Fatal(next.err)
	}
	return next
}
