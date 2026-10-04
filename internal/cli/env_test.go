package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"testing"
)

func TestEnvCommandPrintsTMUXEnvironment(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,100,2")
	t.Setenv("TMUX_PANE", "%7")
	for _, key := range []string{"HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(key, "")
	}
	origOutput, origInspect, origLook := commandOutput, inspectProcess, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if cmd.Args[1] == "display-message" {
			if !slices.Equal(cmd.Args[2:5], []string{"-p", "-t", "%7"}) {
				return nil, fmt.Errorf("unexpected target: %v", cmd.Args)
			}
			return []byte("/tmp/tmux-501/default\t100\t$2\t%7\t/dev/ttys003\n"), nil
		}
		return nil, fmt.Errorf("no server listing")
	}
	inspectProcess = func(int) (int, error) { return 0, fmt.Errorf("no process info") }
	lookPath = func(string) (string, error) { return "/usr/bin/tmux", nil }
	t.Cleanup(func() {
		commandOutput, inspectProcess, lookPath = origOutput, origInspect, origLook
		envJSON = false
	})

	cmd, _, err := rootCmd.Find([]string{"env"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd.SetOut(&output)
	t.Cleanup(func() { cmd.SetOut(nil) })
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	want := "TMUX=/tmp/tmux-501/default,100,2\nTMUX_PANE=%7\nTTY=/dev/ttys003\n"
	if output.String() != want {
		t.Errorf("env output = %q, want %q", output.String(), want)
	}

	output.Reset()
	envJSON = true
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var object map[string]string
	if err := json.Unmarshal(output.Bytes(), &object); err != nil {
		t.Fatalf("invalid JSON %q: %v", output.Bytes(), err)
	}
	if object["TMUX_PANE"] != "%7" || object["TTY"] != "/dev/ttys003" || len(object) != 3 {
		t.Errorf("env --json = %v", object)
	}
	if err := cmd.RunE(cmd, []string{"true"}); err == nil {
		t.Error("--json with a command did not fail")
	}
}

func TestTMUXEnvironmentWithoutPane(t *testing.T) {
	got, err := tmuxEnvironment(tmuxContext{session: "/tmp/tmux-501/default,100,3"})
	if err != nil || !slices.Equal(got, []envVar{{"TMUX", "/tmp/tmux-501/default,100,3"}}) {
		t.Errorf("tmuxEnvironment() = %v, %v", got, err)
	}
	if _, err := tmuxEnvironment(tmuxContext{}); err == nil {
		t.Error("empty context did not fail")
	}
}

func TestHerdrEnvironmentIncludesPaneTTY(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "")
	origOutput, origLook := commandOutput, lookPath
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		if !slices.Equal(cmd.Args[1:], []string{"pane", "process-info", "--pane", "w1:p1"}) {
			return nil, fmt.Errorf("unexpected command: %v", cmd.Args)
		}
		if !slices.Contains(cmd.Env, "HERDR_SOCKET_PATH=/run/herdr.sock") {
			return nil, fmt.Errorf("socket not pinned: %v", cmd.Env)
		}
		return []byte(`{"result":{"process_info":{"pane_id":"w1:p1","shell_pid":100,"tty":"/dev/ttys004"}}}`), nil
	}
	lookPath = func(string) (string, error) { return "/usr/local/bin/herdr", nil }
	t.Cleanup(func() { commandOutput, lookPath = origOutput, origLook })

	got, err := herdrEnvironment(herdrContext{socket: "/run/herdr.sock", workspace: "w1", tab: "w1:t1", pane: "w1:p1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []envVar{
		{"HERDR_ENV", "1"},
		{"HERDR_SOCKET_PATH", "/run/herdr.sock"},
		{"HERDR_WORKSPACE_ID", "w1"},
		{"HERDR_TAB_ID", "w1:t1"},
		{"HERDR_PANE_ID", "w1:p1"},
		{"TTY", "/dev/ttys004"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("herdrEnvironment() = %v, want %v", got, want)
	}
}

func TestMergeEnvironmentOverridesExistingVariables(t *testing.T) {
	got := mergeEnvironment([]string{"PATH=/bin", "TMUX=stale", "TMUX_PANE=%1"}, []envVar{{"TMUX", "fresh"}, {"TTY", "/dev/ttys001"}})
	want := []string{"PATH=/bin", "TMUX_PANE=%1", "TMUX=fresh", "TTY=/dev/ttys001"}
	if !slices.Equal(got, want) {
		t.Errorf("mergeEnvironment() = %v, want %v", got, want)
	}
}
