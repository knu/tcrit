package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-shellwords"
)

type editorFinishedMsg struct {
	path string
	err  error
}

func (m *AppModel) openExternalEditor() tea.Cmd {
	file, err := os.CreateTemp("", "tcrit-comment-*.md")
	if err != nil {
		return func() tea.Msg { return errMsg{fmt.Errorf("creating editor file: %w", err)} }
	}
	path := file.Name()
	if _, err := file.WriteString(m.modalTextarea.Value()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{fmt.Errorf("writing editor file: %w", err)} }
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{fmt.Errorf("closing editor file: %w", err)} }
	}

	cmd, err := externalEditorCommand(path)
	if err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return errMsg{err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	})
}

func externalEditorCommand(path string) (*exec.Cmd, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	args, err := shellwords.Parse(editor)
	if err != nil {
		return nil, fmt.Errorf("parsing $EDITOR: %w", err)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("$EDITOR is empty")
	}
	return exec.Command(args[0], append(args[1:], path)...), nil
}

func (m *AppModel) finishExternalEdit(msg editorFinishedMsg) {
	defer func() { _ = os.Remove(msg.path) }()
	if msg.err != nil {
		m.err = fmt.Errorf("running $EDITOR: %w", msg.err)
		return
	}
	content, err := os.ReadFile(msg.path)
	if err != nil {
		m.err = fmt.Errorf("reading editor file: %w", err)
		return
	}
	m.modalTextarea.SetValue(string(content))
	m.modalFocus = 0
	m.modalTextarea.Focus()
}
