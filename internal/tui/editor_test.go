package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExternalEditorCommandParsesArguments(t *testing.T) {
	t.Setenv("EDITOR", `code --wait --profile "Review Profile"`)

	cmd, err := externalEditorCommand("comment.md")
	if err != nil {
		t.Fatalf("externalEditorCommand: %v", err)
	}
	want := []string{"code", "--wait", "--profile", "Review Profile", "comment.md"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("editor args = %#v, want %#v", cmd.Args, want)
	}
}

func TestExternalEditorCommandRejectsMalformedValue(t *testing.T) {
	t.Setenv("EDITOR", `vim "unterminated`)

	if _, err := externalEditorCommand("comment.md"); err == nil {
		t.Fatal("externalEditorCommand accepted malformed $EDITOR")
	}
}

func TestFinishExternalEditReplacesCommentAndRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(path, []byte("edited externally\n"), 0600); err != nil {
		t.Fatalf("writing editor file: %v", err)
	}
	app := NewApp("test.go", AppConfig{})
	app.modal = commentModal
	app.modalTextarea.SetValue("original")

	updated, _ := app.Update(editorFinishedMsg{path: path})
	app = updated.(AppModel)

	if got := app.modalTextarea.Value(); got != "edited externally\n" {
		t.Fatalf("edited comment = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("editor file was not removed: %v", err)
	}
}
